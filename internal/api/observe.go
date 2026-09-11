package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
)

const requestIDHeader = "X-Request-Id"

// infoKey holds the per-request bookkeeping. userKey is 1.
const infoKey ctxKey = 2

// reqInfo is mutable on purpose. The user is resolved by s.auth, which runs
// *inside* observe, and the access line is written while the request is already
// unwinding — a plain value in the context would still say "Guest" for every
// signed-in request ever made.
type reqInfo struct {
	id    string
	user  string
	start time.Time
}

// newRequestID mints the identifier a caller quotes back at support. It is not
// chi's middleware.RequestID: that one copies the inbound header verbatim into
// the log, embeds the container's hostname and a global counter in what it
// generates, and never answers with a header at all.
func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// A probe id is not worth failing a request over.
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

// sanitizeRequestID returns "" for anything it will not repeat.
//
// The value arrives from the client and ends up in a log line, in a response
// header and in a text column. Unvalidated, a newline in it forges a log entry
// and a megabyte of it floods the column — so the rule is a conservative
// charset and a hard ceiling, not an escape. The charset still admits UUID,
// ULID and a W3C traceparent, so honouring those later costs nothing.
func sanitizeRequestID(v string) string {
	if len(v) < 8 || len(v) > 64 {
		return ""
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return ""
		}
	}
	return v
}

func infoOf(r *http.Request) *reqInfo {
	i, _ := r.Context().Value(infoKey).(*reqInfo)
	return i
}

// RequestIDOf is the id this request is known by, everywhere it is mentioned.
func RequestIDOf(r *http.Request) string {
	if i := infoOf(r); i != nil {
		return i.id
	}
	return ""
}

// observe gives every request an identity and a duration.
//
// It is the outermost middleware so that a 401 from s.auth and a panic from
// anywhere below both come out carrying the same id the caller was handed.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitizeRequestID(r.Header.Get(requestIDHeader))
		if id == "" {
			id = newRequestID()
		}
		info := &reqInfo{id: id, user: "Guest", start: time.Now()}
		// Set before calling through: headers are flushed on the first write,
		// and /api/events writes immediately and then lives for minutes.
		w.Header().Set(requestIDHeader, id)
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		ctx := context.WithValue(r.Context(), infoKey, info)
		// The same id in the engine's context is what lets LogError correlate
		// from deep inside a handler without another parameter.
		ctx = engine.WithRequestID(ctx, id)
		defer func() { s.logRequest(info, r, ww) }()
		next.ServeHTTP(ww, r.WithContext(ctx))
	})
}

// logRequest writes at most one line per request.
//
// Only r.URL.Path is logged, never the raw query: a list filter carries
// personal data and a recovery link carries a token, and neither belongs in a
// file that gets shipped to a collector and kept.
func (s *Server) logRequest(info *reqInfo, r *http.Request, ww middleware.WrapResponseWriter) {
	d := time.Since(info.start)
	status := ww.Status()
	if status == 0 {
		status = http.StatusOK // handler returned without ever writing
	}
	level := logLevelFor(r.URL.Path, status, d, s.E.Cfg.Ops.SlowRequest())
	if !s.E.Log.Enabled(r.Context(), level) {
		return
	}
	s.E.Log.Log(r.Context(), level, "request",
		"id", info.id, "method", r.Method, "path", r.URL.Path,
		"status", status, "ms", d.Milliseconds(), "user", info.user, "ip", clientIP(r))
}

// logLevelFor keeps the access log readable. One page of the desk is ~30
// requests through this same chain — the SPA fallback, every asset, every
// file — so logging all of them at info buries the handful of lines that say
// something. An asset that answered 200 quickly is noise; the same asset
// answering 500, or taking two seconds, is not.
func logLevelFor(path string, status int, d, slow time.Duration) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	// SSE legitimately lives for minutes: its duration says nothing about
	// whether the server is slow.
	case d > slow && path != "/api/events":
		return slog.LevelWarn
	case isProbePath(path):
		// A probe runs every few seconds forever. It has a status code an
		// orchestrator already acts on; it does not need a log line too.
		return slog.LevelDebug
	case strings.HasPrefix(path, "/api/"), strings.HasPrefix(path, "/mcp"):
		return slog.LevelInfo
	default:
		// assets, /files, and the desk's own SPA fallback
		return slog.LevelDebug
	}
}

// recoverPanic replaces chi's Recoverer.
//
// Chi's writes a bare 500 with no body: the desk then shows a toast built from
// a slice of plain text, nothing reaches Error Log, and the request id the
// caller was given leads nowhere. A panic is the failure most worth being able
// to look up afterwards, so it gets the same envelope as any other error.
func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// chi and net/http both treat this as "the handler decided to stop
			// abruptly": re-panicking is what keeps SSE and hijacked
			// connections behaving.
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec)
			}
			id := RequestIDOf(r)
			s.E.Log.Error("panic", "id", id, "method", r.Method, "path", r.URL.Path,
				"panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
			// The Error Log row is a database write, and a database write can
			// panic too. A panic inside the panic handler takes the whole
			// process down, so it is contained here.
			func() {
				defer func() { _ = recover() }()
				s.E.LogError(r.Context(), "api.panic", fmt.Errorf("panic: %v", rec))
			}()
			// Only if nothing reached the wire yet: a half-written response
			// cannot be turned into a JSON error, and trying corrupts it.
			if ww, ok := w.(middleware.WrapResponseWriter); ok && ww.BytesWritten() > 0 {
				return
			}
			// The panic text stays in the log and in the Error Log row; the
			// caller gets the id, which is what joins the two.
			s.writeError(w, r, cerr.Internal("Unexpected server error"), false)
		}()
		next.ServeHTTP(w, r)
	})
}
