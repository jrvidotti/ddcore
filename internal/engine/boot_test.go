package engine

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

// onBoot runs once per process, each app in a transaction of its own: one
// that throws is logged and does not keep the next from running, and neither
// a second call nor a reload of the definitions runs it again (#126).
func TestOnBootRunsOncePerProcess(t *testing.T) {
	broken := writeApp(t, map[string]string{
		"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "broken", title: "Broken", onBoot() { throw new Error("provider is down"); } });`,
	})
	boot := writeApp(t, map[string]string{
		"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "boot", title: "Boot",
  onBoot() { ddcore.newDoc("Boot Mark", { valor: ddcore.env("boot_valor") || "unset" }).insert(); } });`,
		"doctypes/boot_mark/boot_mark.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Boot Mark", fields: [{ fieldname: "valor", fieldtype: "Data", label: "Value" }] });`,
	})
	e := migratedEngine(t, Config{Apps: []js.App{
		{Name: "demo", Dir: testApp(t)}, {Name: "broken", Dir: broken}, {Name: "boot", Dir: boot},
	}, Test: true})
	var logs bytes.Buffer
	e.Log = slog.New(slog.NewTextHandler(&logs, nil))
	t.Setenv("DDCORE_APP_BOOT_VALOR", "from env")
	ctx := context.Background()

	marks := func() []map[string]any {
		var out []map[string]any
		if err := e.Run(ctx, "Admin", func(c *Ctx) error {
			var err error
			out, err = c.GetList("Boot Mark", ListArgs{Fields: []string{"valor"}})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}

	if got := marks(); len(got) != 0 {
		t.Fatalf("onBoot ran before Boot (migrate or load): %v", got)
	}
	e.Boot(ctx)
	got := marks()
	if len(got) != 1 || got[0]["valor"] != "from env" {
		t.Fatalf("after Boot: %v", got)
	}
	if !strings.Contains(logs.String(), "onBoot failed") || !strings.Contains(logs.String(), "app=broken") {
		t.Errorf("the failing hook was not logged:\n%s", logs.String())
	}

	e.Boot(ctx)
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}
	e.Boot(ctx)
	if got := marks(); len(got) != 1 {
		t.Fatalf("onBoot ran again: %v", got)
	}
}
