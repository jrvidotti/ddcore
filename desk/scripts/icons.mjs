// Adds lucide icons to the desk's icon set: `npm run icons:add -- wallet coins`.
//
// The paths come from lucide-static (a pinned devDependency), each SVG element
// turned into the one `d` the desk draws it with. A name already in the table,
// as an icon or an alias, is left as it is. icons/icons.json is shared with the
// server, so follow with `GOWORK=off go test ./desk/icons -update` to refresh
// the list in docs/agent/report-api.md.
import { readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";

const tablePath = new URL("../icons/icons.json", import.meta.url);
const require = createRequire(import.meta.url);
const nodes = require("lucide-static/icon-nodes.json");

const n = (v) => Number(v);
// A number as lucide writes it: no trailing zeros, no "-0".
const f = (v) => String(Math.round(v * 1000) / 1000 + 0);

function circle(cx, cy, rx, ry) {
  return `M${f(cx - rx)} ${f(cy)}a${f(rx)} ${f(ry)} 0 1 0 ${f(2 * rx)} 0a${f(rx)} ${f(ry)} 0 1 0 ${f(-2 * rx)} 0`;
}

function rect(a) {
  const x = n(a.x ?? 0), y = n(a.y ?? 0), w = n(a.width), h = n(a.height);
  let rx = n(a.rx ?? a.ry ?? 0), ry = n(a.ry ?? a.rx ?? 0);
  rx = Math.min(rx, w / 2);
  ry = Math.min(ry, h / 2);
  if (!rx || !ry) return `M${f(x)} ${f(y)}h${f(w)}v${f(h)}h${f(-w)}z`;
  const arc = (dx, dy) => `a${f(rx)} ${f(ry)} 0 0 1 ${f(dx)} ${f(dy)}`;
  return `M${f(x + rx)} ${f(y)}h${f(w - 2 * rx)}${arc(rx, ry)}v${f(h - 2 * ry)}${arc(-rx, ry)}` +
    `h${f(-(w - 2 * rx))}${arc(-rx, -ry)}v${f(-(h - 2 * ry))}${arc(rx, -ry)}z`;
}

function points(s, close) {
  const p = s.trim().split(/[\s,]+/).map(n);
  let d = `M${f(p[0])} ${f(p[1])}`;
  for (let i = 2; i < p.length; i += 2) d += `L${f(p[i])} ${f(p[i + 1])}`;
  return close ? d + "z" : d;
}

function toPath(name, [tag, a]) {
  switch (tag) {
    case "path": return a.d;
    case "circle": return circle(n(a.cx), n(a.cy), n(a.r), n(a.r));
    case "ellipse": return circle(n(a.cx), n(a.cy), n(a.rx), n(a.ry));
    case "rect": return rect(a);
    case "line": return `M${f(n(a.x1))} ${f(n(a.y1))}L${f(n(a.x2))} ${f(n(a.y2))}`;
    case "polyline": return points(a.points, false);
    case "polygon": return points(a.points, true);
    default: throw new Error(`icon "${name}": no conversion for <${tag}>`);
  }
}

const names = process.argv.slice(2);
if (!names.length) {
  console.error("usage: npm run icons:add -- <lucide-name> [<lucide-name>...]");
  process.exit(2);
}

// lucide-static ships a file for each old name too, drawing what the icon it
// was renamed to draws; that is how an old name finds its current one.
function renamedTo(name) {
  const svg = (k) => {
    try {
      const s = readFileSync(require.resolve(`lucide-static/icons/${k}.svg`), "utf8");
      return s.slice(s.indexOf(">", s.indexOf("<svg")) + 1, s.indexOf("</svg>")).replace(/\s+/g, " ").trim();
    } catch {
      return null;
    }
  };
  const body = svg(name);
  return body ? Object.keys(nodes).find((k) => svg(k) === body) : undefined;
}

const table = JSON.parse(readFileSync(tablePath, "utf8"));
const unknown = names.filter((name) => !nodes[name] && !table.icons[name] && !table.aliases[name]);
if (unknown.length) {
  for (const name of unknown) {
    const to = renamedTo(name);
    console.error(to ? `"${name}" is lucide's old name for "${to}": add "${to}", and "${name}" to the aliases if apps use it` : `"${name}" is not a lucide icon`);
  }
  console.error(`(lucide-static ${require("lucide-static/package.json").version}; nothing written)`);
  process.exit(1);
}

const added = [];
for (const name of names) {
  if (table.icons[name] || table.aliases[name]) continue;
  table.icons[name] = nodes[name].map((node) => toPath(name, node));
  added.push(name);
}

// The table's own layout: one icon or alias per line, keys sorted.
const block = (obj) => Object.keys(obj).sort().map((k) => `    ${JSON.stringify(k)}: ${JSON.stringify(obj[k]).replaceAll('","', '", "')}`).join(",\n");
writeFileSync(tablePath, `{\n  "icons": {\n${block(table.icons)}\n  },\n  "aliases": {\n${block(table.aliases)}\n  }\n}\n`);
console.log(added.length ? `added ${added.length}: ${added.join(", ")}` : "nothing to add");
