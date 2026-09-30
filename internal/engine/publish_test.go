package engine

import (
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

// #49: publish with { doctype, id } is an event about that document, so the
// hub delivers it only to subscribers who may read it, like doc_update.
func TestPublishNamesItsDocument(t *testing.T) {
	e := &Engine{Events: NewHub()}
	pool, err := js.NewPool(e, nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	c := &Ctx{E: e}
	rt.Ctx = c

	reader := e.Events.Subscribe("ana@x.com", func(doctype, name string) bool { return doctype == "Chat Session" && name == "CS-1" })
	other := e.Events.Subscribe("ze@x.com", func(doctype, name string) bool { return false })
	if _, err := rt.Eval(`ddcore.publish("my_app.chat", {session: "CS-1"}, {doctype: "Chat Session", id: "CS-1"});
ddcore.publish("my_app.all", {}, {});`); err != nil {
		t.Fatal(err)
	}
	for _, f := range c.afterCommit {
		f()
	}
	names := func(ch chan Event) (out []string) {
		for {
			select {
			case ev := <-ch:
				out = append(out, ev.Name)
			default:
				return out
			}
		}
	}
	if got := strings.Join(names(reader), ","); got != "my_app.chat,my_app.all" {
		t.Fatalf("reader got %q", got)
	}
	if got := strings.Join(names(other), ","); got != "my_app.all" {
		t.Fatalf("a user who cannot read the document got %q", got)
	}

	// the name is written into the SSE stream as is: a line break would forge events
	for _, bad := range []string{`"a\ndata: x"`, `""`, `"a b"`} {
		if _, err := rt.Eval(`ddcore.publish(` + bad + `, {})`); err == nil || !strings.Contains(err.Error(), "event name") {
			t.Errorf("publish(%s): want an event name error, got %v", bad, err)
		}
	}
}
