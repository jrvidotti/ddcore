// Package cerr defines the typed errors shared by the core, the JS bridge and the API.
package cerr

import "fmt"

type Error struct {
	Type    string `json:"type"`
	Title   string `json:"title,omitempty"`
	Message string `json:"message"`
	Status  int    `json:"-"`
	Extra   any    `json:"extra,omitempty"`
}

func (e *Error) Error() string {
	if e.Title != "" {
		return e.Title + ": " + e.Message
	}
	return e.Message
}

func New(typ string, status int, msg string, a ...any) *Error {
	return &Error{Type: typ, Status: status, Message: fmt.Sprintf(msg, a...)}
}

func Validation(msg string, a ...any) *Error  { return New("ValidationError", 417, msg, a...) }
func Permission(msg string, a ...any) *Error  { return New("PermissionError", 403, msg, a...) }
func NotFound(msg string, a ...any) *Error    { return New("DoesNotExistError", 404, msg, a...) }
func LinkExists(msg string, a ...any) *Error  { return New("LinkExistsError", 417, msg, a...) }
func Timestamp(msg string, a ...any) *Error   { return New("TimestampMismatchError", 409, msg, a...) }
func Duplicate(msg string, a ...any) *Error   { return New("DuplicateEntryError", 409, msg, a...) }
func Auth(msg string, a ...any) *Error        { return New("AuthenticationError", 401, msg, a...) }
func Internal(msg string, a ...any) *Error    { return New("InternalError", 500, msg, a...) }
func Mandatory(msg string, a ...any) *Error   { return New("MandatoryError", 417, msg, a...) }

func (e *Error) WithTitle(t string) *Error { e.Title = t; return e }

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
