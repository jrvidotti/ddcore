/**
 * What the desk's home shows when it has no workspace to open: "none-declared"
 * when the site has none, "no-access" when it has some and the reader's roles
 * open none of them (boot's `workspacesDenied`, #120), null when there is one.
 */
export type EmptyHome = "no-access" | "none-declared" | null;

export function emptyHome(data: { workspaces?: unknown[]; workspacesDenied?: boolean } | null | undefined): EmptyHome {
  if (!data || data.workspaces?.length) return null;
  return data.workspacesDenied ? "no-access" : "none-declared";
}

/** The account as the reader knows it: the name, with the login beside it when it differs. */
export function accountLabel(user: string, fullName?: string | null): string {
  const name = (fullName || "").trim();
  return name && name !== user ? `${name} (${user})` : user;
}
