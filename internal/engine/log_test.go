package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// #88: an object passed to ddcore.log.* was logged as "[object Object]". A plain
// object's keys are now fields of the record, and nothing a caller passes makes
// the call throw.
func TestLogObjectArgumentsBecomeFields(t *testing.T) {
	e := setup(t)
	var buf bytes.Buffer
	orig := e.Log
	e.Log = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { e.Log = orig })

	_, logs, err := e.Eval(context.Background(), `
ddcore.log.info("ctx", { a: 1, b: [2] }, "tail");
ddcore.log.warn({ msg: "shadow", user: "x" });
ddcore.log.error("failed", new Error("boom"), [1, 2], null);
const loop: any = { n: 1 }; loop.self = loop;
console.log("cycle", loop, { err: new Error("inner") });
1`, false)
	if err != nil {
		t.Fatal(err)
	}
	var recs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		recs = append(recs, r)
	}
	if len(recs) != 4 {
		t.Fatalf("want 4 records, got %d:\n%s", len(recs), buf.String())
	}
	check := func(i int, key string, want any) {
		t.Helper()
		got, _ := json.Marshal(recs[i][key])
		w, _ := json.Marshal(want)
		if string(got) != string(w) {
			t.Errorf("record %d: %s = %s, want %s", i, key, got, w)
		}
	}
	check(0, "msg", "ctx tail")
	check(0, "a", 1)
	check(0, "b", []any{2})
	check(0, "user", "Admin")
	check(1, "msg", "")
	check(1, "arg.msg", "shadow")
	check(1, "arg.user", "x")
	check(1, "user", "Admin")
	check(2, "level", "ERROR")
	check(2, "msg", "failed Error: boom [1,2] null")
	check(3, "msg", "cycle")
	check(3, "n", 1)
	check(3, "self", "[object Object]")
	check(3, "err", "Error: inner")

	if len(logs) != 4 || logs[0] != `info: ctx tail {"a":1,"b":[2]}` || logs[1] != `warn: {"msg":"shadow","user":"x"}` {
		t.Fatalf("captured logs: %q", logs)
	}
}
