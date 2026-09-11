package engine

import (
	"context"
	"log/slog"
	"os"
)

// The request identifier travels in the context because the code that needs it
// last — LogError, writing a row an operator will grep — is several calls away
// from the HTTP border that minted it, and every layer in between (Ctx, the JS
// bridge, a job) would otherwise have to carry a parameter it never reads.
//
// It lives here, and not in internal/api, for a plainer reason: LogError is an
// engine method and the engine cannot import the api package.
type reqIDKey struct{}

// WithRequestID marks ctx as belonging to one request. The api border calls it
// once per request; a job calls it with its own handle so a failed run is
// locatable too.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, reqIDKey{}, id)
}

// RequestIDFrom is empty when the work has no request behind it — a migration,
// a test, a CLI command. That emptiness is information: an Error Log row with
// no id did not come from anyone's browser.
func RequestIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(reqIDKey{}).(string)
	return id
}

// logHandler picks the shape and the destination of the log. Text is what a
// person reads over a terminal while developing; JSON is what a collector can
// index, and an access line nobody can index is an access line that correlates
// nothing.
//
// The default destination is stdout and not stderr, slog's own default: a
// platform that captures both streams — Railway, Cloud Run, Kubernetes —
// reads a line on stderr as an error by definition, so an INFO access line
// arrives painted red and the level policy stops meaning anything. stderr is
// left to the process that really has nothing else (a crash before the engine
// exists) and to the command whose stdout carries a protocol.
func logHandler(cfg Config) slog.Handler {
	o := &slog.HandlerOptions{Level: cfg.LogLevel}
	w := cfg.LogOut
	if w == nil {
		w = os.Stdout
	}
	if cfg.LogJSON {
		return slog.NewJSONHandler(w, o)
	}
	return slog.NewTextHandler(w, o)
}
