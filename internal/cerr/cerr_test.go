package cerr

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// The whole point of the redesign: constructing an error must not lose the
// template, because the border needs it to translate.
func TestNewKeepsKeyAndArgs(t *testing.T) {
	e := Validation("{0}: value \"{1}\" is not one of the options", "Status", "Aberto")
	if e.Key != "{0}: value \"{1}\" is not one of the options" {
		t.Fatalf("Key = %q", e.Key)
	}
	if !reflect.DeepEqual(e.Args, []any{"Status", "Aberto"}) {
		t.Fatalf("Args = %v", e.Args)
	}
	if want := `Status: value "Aberto" is not one of the options`; e.Message != want {
		t.Fatalf("Message = %q, want %q", e.Message, want)
	}
	if e.Type != "ValidationError" || e.Status != 417 {
		t.Fatalf("Type/Status = %q/%d", e.Type, e.Status)
	}
	// Error() still works for logs, the CLI and errors.As
	if e.Error() != e.Message {
		t.Fatalf("Error() = %q", e.Error())
	}
	var target *Error
	if !errors.As(error(e), &target) || target != e {
		t.Fatal("errors.As no longer finds *Error")
	}
}

func TestRender(t *testing.T) {
	cases := []struct {
		s    string
		args []any
		want string
	}{
		{"no placeholders", []any{"x"}, "no placeholders"},
		{"{0} and {1}", []any{"a", "b"}, "a and b"},
		{"{1} before {0}", []any{"a", "b"}, "b before a"},
		{"{0} twice {0}", []any{"a"}, "a twice a"},
		{"{0} of {1}", []any{"a"}, "a of {1}"}, // a missing argument keeps its placeholder
		{"{0} rows", []any{3}, "3 rows"},
		{"{0}", nil, "{0}"},
	}
	for _, c := range cases {
		if got := Render(c.s, c.args); got != c.want {
			t.Errorf("Render(%q, %v) = %q, want %q", c.s, c.args, got, c.want)
		}
	}
}

func TestTranslateAtTheBorder(t *testing.T) {
	dict := map[string]string{
		"{0} {1} not found": "{0} {1} não encontrado",
		"Not found":         "Não encontrado",
	}
	tr := func(key string, args ...any) string {
		if t, ok := dict[key]; ok {
			key = t
		}
		return Render(key, args)
	}

	e := NotFound("{0} {1} not found", "Task", "TAR-0001").WithTitleKey("Not found")
	got := e.Translate(tr)
	if got.Message != "Task TAR-0001 não encontrado" {
		t.Fatalf("Message = %q", got.Message)
	}
	if got.Title != "Não encontrado" {
		t.Fatalf("Title = %q", got.Title)
	}
	// the original is untouched: a *State is shared, an error is not a place
	// to keep per-request text
	if e.Message != "Task TAR-0001 not found" || e.Title != "Not found" {
		t.Fatalf("original mutated: %q / %q", e.Message, e.Title)
	}
}

// An error raised inside the JS runtime went through `_()` there, so it
// arrives with no key and must be passed through untouched.
func TestTranslateSkipsKeylessErrors(t *testing.T) {
	e := &Error{Type: "ValidationError", Message: "já traduzido na VM", Status: 417}
	got := e.Translate(func(string, ...any) string { return "SHOULD NOT HAPPEN" })
	if got.Message != "já traduzido na VM" {
		t.Fatalf("Message = %q", got.Message)
	}
}

// The goja bridge re-throws a Go error as JSON and parses it back; key and
// args have to survive that round trip.
func TestJSONRoundTrip(t *testing.T) {
	e := Duplicate("{0} \"{1}\" already exists", "Code", "P-1")
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var back Error
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Key != e.Key {
		t.Fatalf("Key = %q", back.Key)
	}
	if !reflect.DeepEqual(back.Args, []any{"Code", "P-1"}) {
		t.Fatalf("Args = %v", back.Args)
	}
	if back.Message != e.Message {
		t.Fatalf("Message = %q", back.Message)
	}
}

func TestFrom(t *testing.T) {
	if From(nil) != nil {
		t.Fatal("From(nil) should be nil")
	}
	e := Auth("no")
	if From(e) != e {
		t.Fatal("From should pass an *Error through")
	}
	if got := From(errors.New("plain")); got.Type != "InternalError" || got.Status != 500 || got.Key != "" {
		t.Fatalf("From(plain) = %+v", got)
	}
}

// StatusOf is the table the JS bridge answers by, so it has to agree with
// every constructor: a type raised in Go and the same type thrown from an app
// must answer the same status (#89).
func TestStatusOfAgreesWithTheConstructors(t *testing.T) {
	for _, e := range []*Error{
		Validation("x"), Permission("x"), NotFound("x"), LinkExists("x"), Timestamp("x"),
		Duplicate("x"), Auth("x"), Internal("x"), Mandatory("x"), MethodNotAllowed("x"),
		TooMany("x"), Unavailable("x"), Maintenance("x"),
	} {
		n, ok := StatusOf(e.Type)
		if !ok || n != e.Status {
			t.Errorf("StatusOf(%q) = %d, %v; the constructor answers %d", e.Type, n, ok, e.Status)
		}
	}
	if n, ok := StatusOf("NotFound"); !ok || n != 404 {
		t.Errorf("StatusOf(NotFound) = %d, %v", n, ok)
	}
	if _, ok := StatusOf("PaymentDeclinedError"); ok {
		t.Error("an app's own type is not in the table")
	}
}

// An error thrown from the JS runtime carries its numbers as int64 (goja) or
// float64 (JSON), and its wait must still reach the Retry-After header.
func TestRetryAfterReadsEveryNumberType(t *testing.T) {
	cases := []struct {
		extra any
		want  int
		ok    bool
	}{
		{map[string]any{"retryAfter": 30}, 30, true},
		{map[string]any{"retryAfter": int64(45)}, 45, true},
		{map[string]any{"retryAfter": float64(60)}, 60, true},
		{map[string]any{"retryAfter": 1.2}, 2, true}, // whole seconds, rounded up
		{map[string]any{"retryAfter": -1.0}, 0, false},
		{map[string]any{"retryAfter": int64(-5)}, 0, false},
		{map[string]any{"retryAfter": "60"}, 0, false},
		{map[string]any{"code": "X"}, 0, false},
		{"not a map", 0, false},
		{nil, 0, false},
	}
	for _, c := range cases {
		n, ok := (&Error{Extra: c.extra}).RetryAfter()
		if n != c.want || ok != c.ok {
			t.Errorf("RetryAfter(%v) = %d, %v; want %d, %v", c.extra, n, ok, c.want, c.ok)
		}
	}
	if n, ok := TooMany("x").WithRetryAfter(7).RetryAfter(); n != 7 || !ok {
		t.Errorf("WithRetryAfter round trip = %d, %v", n, ok)
	}
}
