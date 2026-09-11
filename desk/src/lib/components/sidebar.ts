/**
 * What the sidebar decides, as opposed to what it draws — here for the reason
 * `profile.ts` is: a component cannot be unit-tested in this repo, and this is
 * the part worth testing.
 */
import type { Boot } from "$lib/boot.svelte";

/** One entry of the System group: the DocType name and what the boot said about it. */
export type SystemDoctype = [string, { label: string; app: string; icon: string }];

/**
 * The core DocTypes listed under "System".
 *
 * The group is an administration shortcut, not a permission: the boot payload
 * already omits what the user cannot read, but a person without System Manager
 * has no business being pointed at User, Role or Error Log in the first place,
 * so the whole group is theirs only. An empty list hides the heading — a
 * caption over nothing reads as a broken menu.
 */
export function systemDoctypes(data: Boot | null): SystemDoctype[] {
  if (!data || !data.roles?.includes("System Manager")) return [];
  return Object.entries(data.doctypes || {})
    .filter(([, d]) => d.app === "core")
    .sort() as SystemDoctype[];
}
