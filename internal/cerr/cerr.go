// Package cerr defines the typed errors shared by the core, the JS bridge and the API.
package cerr

import (
	"fmt"
	"math"
	"strings"
)

// Error carries both the rendered message and the material to render it
// again in another language.
//
// Key is the English template, with `{0}`-style placeholders — the same key
// the catalogue is written in. Message is that template rendered with Args,
// which for an untranslated error is English. Both travel on the wire: the
// desk shows Message, and a consumer that is not the desk (curl, the MCP
// server, an Error Log row) still reads a complete sentence, while Key and
// Args stay available for telemetry and grouping.
type Error struct {
	Type     string `json:"type"`
	Title    string `json:"title,omitempty"` // already translated on the wire
	Message  string `json:"message"`         // already translated on the wire
	Key      string `json:"key,omitempty"`   // English template, with {0}
	Args     []any  `json:"args,omitempty"`
	TitleKey string `json:"titleKey,omitempty"`
	Status   int    `json:"-"`
	Extra    any    `json:"extra,omitempty"`
	// RequestID is stamped at the HTTP border, never by the code that raised
	// the error. It is the handle a user reads off a red toast and an operator
	// greps for in the log and in Error Log — the one string that joins the
	// three. Empty outside HTTP.
	RequestID string `json:"requestId,omitempty"`
}

func (e *Error) Error() string {
	if e.Title != "" {
		return e.Title + ": " + e.Message
	}
	return e.Message
}

// New builds an error from a message template and its arguments. It renders
// the template but does not lose it: translating at the border needs the key,
// and rendering here keeps every caller that only ever reads Message — the
// CLI, the jobs, `errors.As` — working unchanged.
func New(typ string, status int, msg string, a ...any) *Error {
	return &Error{Type: typ, Status: status, Key: msg, Args: a, Message: Render(msg, a)}
}

// Render substitutes `{0}`, `{1}`, … in s with args. An index with no
// argument keeps its placeholder rather than disappearing: a message missing
// a value should look wrong, not look complete and say the wrong thing.
//
// This is the single implementation of the interpolation; engine.I18n.T
// translates and then calls it.
func Render(s string, args []any) string {
	if len(args) == 0 || !strings.ContainsRune(s, '{') {
		return s
	}
	for n, a := range args {
		s = strings.ReplaceAll(s, fmt.Sprintf("{%d}", n), fmt.Sprint(a))
	}
	return s
}

func Validation(msg string, a ...any) *Error { return New("ValidationError", 417, msg, a...) }
func Permission(msg string, a ...any) *Error { return New("PermissionError", 403, msg, a...) }
func NotFound(msg string, a ...any) *Error   { return New("DoesNotExistError", 404, msg, a...) }
func LinkExists(msg string, a ...any) *Error { return New("LinkExistsError", 417, msg, a...) }
func Timestamp(msg string, a ...any) *Error  { return New("TimestampMismatchError", 409, msg, a...) }
func Duplicate(msg string, a ...any) *Error  { return New("DuplicateEntryError", 409, msg, a...) }
func Auth(msg string, a ...any) *Error       { return New("AuthenticationError", 401, msg, a...) }
func Internal(msg string, a ...any) *Error   { return New("InternalError", 500, msg, a...) }
func Mandatory(msg string, a ...any) *Error  { return New("MandatoryError", 417, msg, a...) }

// MethodNotAllowed answers a verb a whitelisted method did not declare.
func MethodNotAllowed(msg string, a ...any) *Error {
	return New("MethodNotAllowedError", 405, msg, a...)
}

// TooMany is the answer to a caller that has to slow down: a login being
// guessed at, a recovery being requested in a loop. The seconds still to wait
// belong in Extra through WithRetryAfter, because the number is data the
// caller acts on, not prose it reads.
func TooMany(msg string, a ...any) *Error     { return New("TooManyRequestsError", 429, msg, a...) }
func Unavailable(msg string, a ...any) *Error { return New("UnavailableError", 503, msg, a...) }

// Maintenance refuses a write while the site is paused for a cutover, a backup
// or a restore. It is a 503 like Unavailable, but a type of its own, so the
// desk can tell "try again after the window" from "the server is broken".
func Maintenance(msg string, a ...any) *Error { return New("MaintenanceError", 503, msg, a...) }

// WithRetryAfter records how many seconds the caller must wait. The API
// border turns it into the `Retry-After` header, and it travels in the body
// too, so a client that never reads headers can still count down.
func (e *Error) WithRetryAfter(seconds int) *Error {
	e.Extra = map[string]any{"retryAfter": seconds}
	return e
}

// RetryAfter reads back the seconds in Extra's "retryAfter". The second
// result is false when this error carries no wait at all.
//
// WithRetryAfter stores an int, but an error raised in the JS runtime reaches
// Go with its numbers as int64 (goja's export) or float64 (a JSON round
// trip), so all three are read. A fraction rounds up, since Retry-After
// counts whole seconds and an early retry would be refused again; a negative
// or non-finite wait is no wait.
func (e *Error) RetryAfter() (int, bool) {
	m, ok := e.Extra.(map[string]any)
	if !ok {
		return 0, false
	}
	var f float64
	switch n := m["retryAfter"].(type) {
	case int:
		f = float64(n)
	case int64:
		f = float64(n)
	case float64:
		f = n
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0, false
	}
	return int(math.Ceil(min(f, math.MaxInt32))), true
}

// WithTitle sets an already-translated title.
func (e *Error) WithTitle(t string) *Error { e.Title = t; return e }

// WithTitleKey sets a title the border still has to translate — the same
// contract Key has for the message.
func (e *Error) WithTitleKey(k string) *Error { e.TitleKey = k; e.Title = k; return e }

// Translate returns a copy rendered through tr, which takes a key and its
// arguments and returns the translated, interpolated string. An error with no
// Key came from somewhere that already translated it — `ddcore.throw(_("…"))`
// inside the JS runtime — and is returned untouched.
func (e *Error) Translate(tr func(key string, args ...any) string) *Error {
	if e == nil || e.Key == "" {
		return e
	}
	out := *e
	out.Message = tr(e.Key, e.Args...)
	if e.TitleKey != "" {
		out.Title = tr(e.TitleKey)
	}
	return &out
}

// statusByType is the HTTP status of every error type this package defines —
// the one table both the constructors above and the JS bridge answer by.
var statusByType = map[string]int{
	"ValidationError":        417,
	"MandatoryError":         417,
	"LinkExistsError":        417,
	"PermissionError":        403,
	"DoesNotExistError":      404,
	"NotFound":               404, // the short spelling the JS runtime also accepts
	"TimestampMismatchError": 409,
	"DuplicateEntryError":    409,
	"AuthenticationError":    401,
	"MethodNotAllowedError":  405,
	"TooManyRequestsError":   429,
	"UnavailableError":       503,
	"MaintenanceError":       503,
	"InternalError":          500,
}

// StatusOf returns the HTTP status an error type answers with. The second
// result is false for a type this package does not define; the caller then
// answers 500, the status of an error nobody anticipated.
func StatusOf(typ string) (int, bool) {
	n, ok := statusByType[typ]
	return n, ok
}

// From converts any error into *Error.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return &Error{Type: "InternalError", Status: 500, Message: err.Error()}
}
