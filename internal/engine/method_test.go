package engine

import (
	"context"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// TestCallWhitelisted pins the contract /api/method and the MCP call_method
// tool share (#141): a function not exported with whitelisted() does not
// exist for the caller, and roles refuses a user holding none of them.
func TestCallWhitelisted(t *testing.T) {
	e := setupWith(t, map[string]string{"services/calls.ts": `import { whitelisted } from "@ddcore/sdk";
export function internal() { return "internal"; }
export const open = whitelisted((args: any) => ({ echo: args.x }));
export const managers = whitelisted(() => "managers", { roles: ["Gestor"] });`})
	ctx := context.Background()
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		u, _ := c.NewDoc("User", Doc{"email": "clerk@x.com", "full_name": "Clerk"})
		_, err := c.Insert(u, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	call := func(user, path string) (string, error) {
		var out string
		err := e.Run(ctx, user, func(c *Ctx) error {
			res, err := c.CallWhitelisted(path, []byte(`{"x":7}`))
			out = string(res)
			return err
		})
		return out, err
	}

	if out, err := call("Admin", "demo.services.calls.open"); err != nil || out != `{"echo":7}` {
		t.Fatalf("whitelisted: %q %v", out, err)
	}
	if _, err := call("Admin", "demo.services.calls.internal"); err == nil || cerr.From(err).Type != "DoesNotExistError" {
		t.Fatalf("a function not whitelisted must not be reachable, even by Admin: %v", err)
	}
	if out, err := call("Admin", "demo.services.calls.managers"); err != nil || out != `"managers"` {
		t.Fatalf("Admin passes roles: %q %v", out, err)
	}
	if _, err := call("clerk@x.com", "demo.services.calls.managers"); err == nil || cerr.From(err).Type != "PermissionError" {
		t.Fatalf("a user without the role must be refused: %v", err)
	}
}
