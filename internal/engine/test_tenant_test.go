package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

// writeApp writes files into a fresh app directory.
func writeApp(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, src := range files {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// scratchRH is an app whose DocTypes live inside a tenant, so its tests run in
// the scratch tenant without saying so; onTenantCreate seeds every tenant.
var scratchRH = map[string]string{
	"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "rh", title: "RH", space: "tenant", roles: ["RH User"],
  onTenantCreate() { ddcore.newDoc("Ponto", { hora: "seed" }).insert(); } });`,
	"doctypes/ponto/ponto.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Ponto", fields: [{ fieldname: "hora", fieldtype: "Data", label: "Hora" }],
  permissions: [{ role: "RH User", read: true, write: true, create: true }] });`,
	"doctypes/cargo/cargo.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Cargo", shared: true, fields: [{ fieldname: "titulo", fieldtype: "Data", label: "Titulo" }] });`,
	"doctypes/ponto/ponto.test.ts": `import "@ddcore/sdk/test";
const pontos = () => ddcore.db.getList("Ponto", { fields: ["hora"], orderBy: "hora asc" }).map((r: any) => r.hora);

describe("rh, in the scratch tenant", () => {
  beforeAll(() => { ddcore.newDoc("Ponto", { hora: "before-all" }).insert(); });

  it("works inside a tenant made for the run", () => {
    expect(ddcore.tenant.current()).toMatch(/^ddcore-test-[0-9a-f]{8}$/);
    ddcore.newDoc("Ponto", { hora: "08:00" }).insert();
    expect(pontos()).toEqual(["08:00", "before-all", "seed"]);
  });

  it("rolls each test back", () => {
    expect(pontos()).toEqual(["before-all", "seed"]);
  });

  it("reaches the platform space with inPlatform, and comes back", () => {
    const here = ddcore.tenant.current();
    const seen = ddcore.test.inPlatform(() => {
      ddcore.newDoc("Cargo", { titulo: "Analista" }).insert();
      expect(() => ddcore.db.count("Ponto")).toThrow("lives inside a tenant");
      return ddcore.tenant.current();
    });
    expect(seen).toBe("");
    expect(ddcore.tenant.current()).toBe(here);
    expect(ddcore.db.count("Cargo")).toBe(1);
  });

  it("does not stay in the platform space after a throw inside inPlatform", () => {
    expect(() => ddcore.test.inPlatform(() => { throw new Error("boom"); })).toThrow("boom");
    expect(ddcore.tenant.current()).toMatch(/^ddcore-test-/);
  });

  it("acts as a user of the scratch tenant", () => {
    ddcore.newDoc("User", { email: "ana@rh.test", full_name: "Ana", roles: [{ role: "RH User" }] }).insert();
    ddcore.test.asUser("ana@rh.test", () => {
      ddcore.newDoc("Ponto", { hora: "09:00" }).insert();
      expect(ddcore.session.user).toBe("ana@rh.test");
    });
    expect(ddcore.db.count("Ponto", { hora: "09:00" })).toBe(1);
  });
});`,
}

// scratchTreino is an app of the platform's own that uses rh's DocTypes, and
// so asks for the scratch tenant.
var scratchTreino = map[string]string{
	"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "treino", title: "Treino", requires: ["rh"], tests: { space: "tenant" } });`,
	"doctypes/curso/curso.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Curso", fields: [{ fieldname: "ponto", fieldtype: "Link", label: "Ponto", options: "Ponto" }] });`,
	"doctypes/curso/curso.test.ts": `import "@ddcore/sdk/test";
describe("treino", () => {
  it("links to a tenant-only DocType", () => {
    const p = ddcore.newDoc("Ponto", { hora: "10:00" }).insert();
    ddcore.newDoc("Curso", { ponto: p.id }).insert();
    expect(ddcore.db.count("Curso")).toBe(1);
  });
});`,
}

// scratchPlain is an app that says nothing: its tests stay in the platform.
var scratchPlain = map[string]string{
	"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "plain", title: "Plain" });`,
	"doctypes/nota/nota.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Nota", fields: [{ fieldname: "texto", fieldtype: "Data", label: "Texto" }] });`,
	"doctypes/nota/nota.test.ts": `import "@ddcore/sdk/test";
describe("plain", () => {
  it("runs in the platform space", () => {
    expect(ddcore.tenant.current()).toBe("");
    ddcore.newDoc("Nota", { texto: "x" }).insert();
  });
  it("still has inPlatform", () => { expect(ddcore.test.inPlatform(() => 7)).toBe(7); });
});`,
}

func scratchApps(t *testing.T) []js.App {
	return []js.App{
		{Name: "plain", Dir: writeApp(t, scratchPlain)},
		{Name: "rh", Dir: writeApp(t, scratchRH)},
		{Name: "treino", Dir: writeApp(t, scratchTreino)},
	}
}

func wantAllPassed(t *testing.T, results []js.TestResult, n int) {
	t.Helper()
	if len(results) != n {
		t.Errorf("%d results, want %d: %+v", len(results), n, results)
	}
	for _, r := range results {
		if !r.OK {
			t.Errorf("%s: %s\n%s", r.Name, r.Error, r.Stack)
		}
	}
}

func TestScratchTenantRunsTheTestsOfATenantApp(t *testing.T) {
	e := migratedEngine(t, Config{Apps: scratchApps(t), Test: true, Tenancy: true})
	ctx := context.Background()
	results, err := e.RunTests(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	wantAllPassed(t, results, 8)
	// the run rolled back: the tenant, its seed and the tests' rows are gone
	for _, table := range []string{"tab_site_tenant", "tab_ponto", "tab_curso", "tab_cargo", "tab_nota"} {
		if got := sysScalar(t, e, "SELECT count(*) FROM "+table); got != 0 {
			t.Errorf("%s keeps %d rows", table, got)
		}
	}
	// one app alone, filtered: still in the scratch tenant
	results, err = e.RunTests(ctx, "", "treino")
	if err != nil {
		t.Fatal(err)
	}
	wantAllPassed(t, results, 1)
}

func TestScratchTenantNeedsTenancy(t *testing.T) {
	e := migratedEngine(t, Config{Apps: scratchApps(t), Test: true})
	results, err := e.RunTests(context.Background(), "", "plain")
	if err != nil {
		t.Fatal(err)
	}
	wantAllPassed(t, results, 2)
}

func TestScratchTenantTestsSpaceIsChecked(t *testing.T) {
	apps := scratchApps(t)
	apps[0].Dir = writeApp(t, map[string]string{"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "plain", title: "Plain", tests: { space: "tenants" as any } });`})
	_, err := New(context.Background(), Config{Apps: apps, Tenancy: true})
	if err == nil || !strings.Contains(err.Error(), `tests.space "tenants"`) {
		t.Fatalf("got %v", err)
	}
}
