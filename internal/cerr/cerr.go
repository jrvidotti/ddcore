// Package cerr defines the typed errors shared by the core, the JS bridge and the API.
package cerr

import (
	"fmt"
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

// TooMany is the answer to a caller that has to slow down: a login being
// guessed at, a recovery being requested in a loop. The seconds still to wait
// belong in Extra through WithRetryAfter, because the number is data the
// caller acts on, not prose it reads.
func TooMany(msg string, a ...any) *Error { return New("TooManyRequestsError", 429, msg, a...) }

// WithRetryAfter records how many seconds the caller must wait. The API
// border turns it into the `Retry-After` header, and it travels in the body
// too, so a client that never reads headers can still count down.
func (e *Error) WithRetryAfter(seconds int) *Error {
	e.Extra = map[string]any{"retryAfter": seconds}
	return e
}

// RetryAfter reads back what WithRetryAfter stored. The second result is
// false when this error carries no wait at all.
func (e *Error) RetryAfter() (int, bool) {
	m, ok := e.Extra.(map[string]any)
	if !ok {
		return 0, false
	}
	n, ok := m["retryAfter"].(int)
	return n, ok
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
