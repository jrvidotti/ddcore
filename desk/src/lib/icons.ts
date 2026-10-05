// The desk's icon set. icons.json is shared with the server, which refuses at
// load an app `icon` that is not in it; the desk draws every name it knows and
// warns, once per name, about one it does not.
import table from "../../icons/icons.json";

const icons: Record<string, string[]> = table.icons;
const aliases: Record<string, string> = table.aliases;
const warned = new Set<string>();

/** The paths of the icon called name (or of the icon it is an alias of), or null. */
export function iconPaths(name: string): string[] | null {
  return icons[name] ?? icons[aliases[name]] ?? null;
}

/** The paths to draw for name: its own, or a circle — with a console warning — for a name the set lacks. */
export function drawIcon(name: string): string[] {
  const paths = iconPaths(name);
  if (paths) return paths;
  if (name && !warned.has(name)) {
    warned.add(name);
    console.warn(`ddcore: the desk has no icon called "${name}"; it draws a circle instead`);
  }
  return icons.circle;
}
