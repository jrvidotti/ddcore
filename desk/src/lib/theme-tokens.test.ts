import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { describe, expect, it } from "vitest";

// An app's `desk.theme` (#128) recolours the desk by redefining the tokens in
// app.css's :root. A colour written as a literal anywhere else stays the
// desk's blue and white whatever the theme says — the bug the issue was about —
// so the desk's styles name tokens, and this test keeps it that way.

const src = join(__dirname, "..");

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f);
    return statSync(p).isDirectory() ? files(p) : p.endsWith(".svelte") || p.endsWith(".css") ? [p] : [];
  });
}

// The literals that are not the brand's, by file: what a theme must not touch.
const kept: Record<string, string[]> = {
  // the toast is dark on every theme
  "app.css": ["#1f2937", "#fff"],
  // the chart's tooltip, dark like the toast
  "lib/components/BarChart.svelte": ["#1f2937", "#fff"],
  // the printed page is paper
  "lib/components/PrintView.svelte": ["#ffffff"],
  // a barcode needs a white margin to scan, and the camera's video is black
  "lib/controls/BarcodeControl.svelte": ["#000", "#fff"],
  // the colour picker draws colours, not the desk
  "lib/controls/ColorControl.svelte": ["#000", "#fff", "#f00", "#ff0", "#0f0", "#0ff", "#00f", "#f0f", "#d1d5db"],
  // the Leaflet marker is set from JS, where CSS variables do not reach
  "lib/controls/GeolocationControl.svelte": ["#2563eb", "#fff"],
  // a signature is ink on paper
  "lib/controls/SignatureControl.svelte": ["#fff", "#111"],
};

// comments go, and app.css's :root, where the tokens are defined
function styles(file: string, text: string): string {
  let s = text.replace(/\/\*[\s\S]*?\*\//g, "").replace(/<!--[\s\S]*?-->/g, "").replace(/(^|[^:"'])\/\/[^\n]*/g, "$1");
  if (file === "app.css") s = s.replace(/^:root\s*\{[\s\S]*?\n\}/m, "");
  return s;
}

describe("theme tokens", () => {
  it("leaves no brand colour written as a literal outside app.css's :root", () => {
    const found: string[] = [];
    for (const path of files(src)) {
      const file = relative(src, path);
      const allowed = kept[file] || [];
      const text = styles(file, readFileSync(path, "utf8"));
      for (const [lit] of text.matchAll(/#[0-9a-fA-F]{3,8}\b|rgba?\([^)]*\)|:\s*white\b/g)) {
        // a black shadow or veil darkens whatever is under it, on any theme
        if (/^rgba?\(\s*0[\s,]+0[\s,]+0\b/.test(lit)) continue;
        if (allowed.includes(lit.toLowerCase())) continue;
        found.push(`${file}: ${lit}`);
      }
    }
    expect(found).toEqual([]);
  });

  it("defines every token the desk's styles read", () => {
    const css = readFileSync(join(src, "app.css"), "utf8");
    const root = css.match(/^:root\s*\{[\s\S]*?\n\}/m)![0];
    const defined = new Set([...root.matchAll(/(--[\w-]+)\s*:/g)].map((m) => m[1]));
    const missing = new Set<string>();
    for (const path of files(src)) {
      for (const [, name, fallback] of readFileSync(path, "utf8").matchAll(/var\((--[\w-]+)\s*(,)?/g)) {
        // a component's own custom property comes with its fallback, or is set inline beside it
        if (!fallback && !defined.has(name) && !readFileSync(path, "utf8").includes(`${name}:`)) missing.add(`${relative(src, path)}: ${name}`);
      }
    }
    expect([...missing]).toEqual([]);
  });
});
