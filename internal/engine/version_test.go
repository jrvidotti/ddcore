package engine

import (
	"context"
	"encoding/json"
	"testing"
)

// #147: ddcore.version was a literal "0.1.0" in the prelude — the default of an
// unstamped build and of every scaffolded app — instead of the core version the
// binary runs, the one `ddcore version`, /api/boot and the MCP server report.
func TestDDCoreVersionIsTheRunningCore(t *testing.T) {
	e := setup(t)
	orig := Version
	Version = "v9.8.7-test"
	t.Cleanup(func() { Version = orig })

	out, _, err := e.Eval(context.Background(), `ddcore.version`, false)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	if got != Version {
		t.Fatalf("ddcore.version = %q, want %q", got, Version)
	}
}
