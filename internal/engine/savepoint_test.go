package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// evalSavepoint runs code in a transaction as Admin and returns its result decoded,
// with the ctx that ran it for a look at what the transaction left behind.
func evalSavepoint(t *testing.T, e *Engine, code string, out any) *Ctx {
	t.Helper()
	var ran *Ctx
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		ran = c
		rt, err := c.RT()
		if err != nil {
			return err
		}
		raw, err := rt.Eval(code)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, out)
	}); err != nil {
		t.Fatal(err)
	}
	return ran
}

// #70: a unique violation inside ddcore.db.savepoint is a DuplicateEntryError
// the app can catch, and the transaction goes on: later statements run, the
// block's messages and events are dropped, and the rest commits.
func TestDBSavepointCatchesADuplicate(t *testing.T) {
	e := setup(t)
	events := e.Events.Subscribe("Admin", nil)
	var got []any
	c := evalSavepoint(t, e, `
ddcore.newDoc("Pessoa", { nome: "Ana", cpf: "1" }).insert();
ddcore.newDoc("Pessoa", { nome: "Bia", cpf: "2" }).insert();
const out = [];
try {
  ddcore.db.savepoint(() => {
    ddcore.msgprint("dropped");
    ddcore.publish("demo.dropped", {});
    ddcore.db.setValue("Pessoa", "Bia", "email", "bia@x.com");
    ddcore.db.setValue("Pessoa", "Bia", "cpf", "1");
  });
  out.push("no error");
} catch (e) {
  out.push(e.name);
}
ddcore.msgprint("kept");
ddcore.publish("demo.kept", {});
ddcore.newDoc("Pessoa", { nome: "Caio", cpf: "3" }).insert();
out.push(ddcore.db.count("Pessoa", {}));
out`, &got)
	if len(got) != 2 || got[0] != "DuplicateEntryError" || got[1] != float64(3) {
		t.Fatalf("got %v, want [DuplicateEntryError 3]", got)
	}
	if len(c.Messages) != 1 || c.Messages[0].Message != "kept" {
		t.Fatalf("messages: %+v", c.Messages)
	}
	var names []string
	for len(events) > 0 {
		if ev := <-events; strings.HasPrefix(ev.Name, "demo.") {
			names = append(names, ev.Name)
		}
	}
	if len(names) != 1 || names[0] != "demo.kept" {
		t.Fatalf("events: %v", names)
	}
	runAs(t, e, "Admin", func(c *Ctx) error {
		bia, err := c.GetDoc("Pessoa", "Bia")
		if err != nil {
			return err
		}
		if bia.Str("cpf") != "2" || bia.Str("email") != "" {
			t.Fatalf("the rolled-back block left a write: %v", bia)
		}
		if ok, err := c.Exists("Pessoa", "Caio"); err != nil || !ok {
			t.Fatalf("the write after the savepoint did not commit: %v %v", ok, err)
		}
		return nil
	})
}

// #70: savepoints nest; a plain throw rolls back the block as a SQL error
// does; a block that returns hands its value back.
func TestDBSavepointNestsAndRollsBackAThrow(t *testing.T) {
	e := setup(t)
	var got []any
	evalSavepoint(t, e, `
ddcore.db.savepoint(() => {
  ddcore.newDoc("Pessoa", { nome: "Outer", cpf: "10" }).insert();
  try {
    ddcore.db.savepoint(() => {
      ddcore.newDoc("Pessoa", { nome: "Inner", cpf: "11" }).insert();
      throw new Error("inner");
    });
  } catch (e) {}
});
try {
  ddcore.db.savepoint(() => {
    ddcore.newDoc("Pessoa", { nome: "Gone", cpf: "12" }).insert();
    ddcore.db.savepoint(() => ddcore.newDoc("Pessoa", { nome: "Gone too", cpf: "13" }).insert());
    ddcore.throw("nope");
  });
} catch (e) {}
const r = ddcore.db.savepoint(() => ({ answer: 42 }));
[ddcore.db.exists("Pessoa", "Outer"), ddcore.db.exists("Pessoa", "Inner"),
 ddcore.db.exists("Pessoa", "Gone"), ddcore.db.exists("Pessoa", "Gone too"), r.answer]`, &got)
	want := []any{"Outer", nil, nil, nil, float64(42)}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// WithSavepoint drops the messages of a block it rolls back, as the JS
// savepoint does, and keeps those of a block that succeeds.
func TestWithSavepointDropsMessages(t *testing.T) {
	e := setup(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		c.Msgprint(Message{Message: "before"})
		c.WithSavepoint(func() error {
			c.Msgprint(Message{Message: "dropped"})
			return errors.New("fail")
		})
		if err := c.WithSavepoint(func() error {
			c.Msgprint(Message{Message: "kept"})
			return nil
		}); err != nil {
			return err
		}
		if len(c.Messages) != 2 || c.Messages[0].Message != "before" || c.Messages[1].Message != "kept" {
			t.Fatalf("messages: %+v", c.Messages)
		}
		return nil
	})
}
