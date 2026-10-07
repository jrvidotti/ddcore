// Which ways in the sign-in screen offers, and which one it opens on (#115).
// Kept apart from the page so it is testable without a DOM.

export interface LoginOffer {
  password?: boolean;
  credentials?: { id: string; label: string }[];
}

/** A tab of the sign-in screen: the e-mail and password form, or an app's credential provider. */
export interface LoginMethod {
  key: string;
  kind: "password" | "credential";
  /** the provider's id, for a credential method */
  id?: string;
  label: string;
}

export const METHOD_KEY = "ddcore.login.method";
export const tenantKey = (provider: string) => "ddcore.login.tenant." + provider;

/**
 * The tabs, in order: the e-mail form when the site offers password sign-in,
 * then one per credential provider. Admin's own form, when password sign-in
 * is off, is not a tab: it is reached through its own link.
 */
export function loginMethods(offer: LoginOffer | undefined, emailLabel: string): LoginMethod[] {
  const out: LoginMethod[] = [];
  if (offer?.password !== false) out.push({ key: "password", kind: "password", label: emailLabel });
  for (const c of offer?.credentials ?? []) {
    out.push({ key: "cred:" + c.id, kind: "credential", id: c.id, label: c.label });
  }
  return out;
}

/** The tab to open: the one used last time, if it is still offered, else the first. */
export function pickMethod(methods: LoginMethod[], remembered?: string | null): string {
  if (remembered && methods.some((m) => m.key === remembered)) return remembered;
  return methods[0]?.key ?? "password";
}

/** The tenant to preselect: the one used last time, else the only one there is. */
export function pickTenant(tenants: { id: string }[], remembered?: string | null): string {
  if (remembered && tenants.some((t) => t.id === remembered)) return remembered;
  return tenants.length === 1 ? tenants[0].id : "";
}

/** localStorage, or nothing: private mode and tests have none. */
export function remember(key: string, value?: string): string {
  try {
    const ls = (globalThis as any).localStorage;
    if (!ls) return "";
    if (value !== undefined) ls.setItem(key, value);
    return ls.getItem(key) || "";
  } catch {
    return "";
  }
}
