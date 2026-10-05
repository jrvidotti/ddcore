import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { drawIcon, iconPaths } from "./icons";

// every .svelte file under src
function svelteFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f);
    return statSync(p).isDirectory() ? svelteFiles(p) : p.endsWith(".svelte") ? [p] : [];
  });
}

describe("icons", () => {
  afterEach(() => vi.restoreAllMocks());

  it("draws every name the desk itself asks for", () => {
    const missing: string[] = [];
    for (const file of svelteFiles(join(__dirname, ".."))) {
      const src = readFileSync(file, "utf8");
      // <Icon name="…">, an icon: "…" property, and the names <Icon name={…}> picks
      // from (a ? "…" : "…", x || "…"), leaving out what it compares against
      const exprs = [...src.matchAll(/<Icon name=(?:("[^"]*")|\{([^}]*)\})/g), ...src.matchAll(/\bicon: ("[^"]*")/g)].map((m) => m[1] ?? m[2]);
      for (const expr of exprs) {
        for (const [, name] of expr.matchAll(/(?:^|[?:|]\s*)"([^"]+)"/g)) {
          if (!iconPaths(name)) missing.push(`${file.slice(file.indexOf("src/"))}: ${name}`);
        }
      }
    }
    expect(missing).toEqual([]);
  });

  it("resolves lucide's other names", () => {
    expect(iconPaths("triangle-alert")).toBe(iconPaths("alert-triangle"));
    expect(iconPaths("chart-column")).toBe(iconPaths("bar-chart-3"));
  });

  it("draws a circle for an unknown name and warns once", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    expect(drawIcon("no-such-icon")).toBe(iconPaths("circle"));
    drawIcon("no-such-icon");
    expect(warn).toHaveBeenCalledTimes(1);
    expect(warn.mock.calls[0][0]).toContain('"no-such-icon"');
    drawIcon("");
    expect(warn).toHaveBeenCalledTimes(1);
  });
});
