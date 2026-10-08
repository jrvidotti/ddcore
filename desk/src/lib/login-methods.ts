// Which ways in the sign-in screen offers, and which one it opens on (#115).
// Kept apart from the page so it is testable without a DOM.

export interface LoginOffer {
  password?: boolean;
  /** the site has tenants: a credential provider asks for the organization first */
  tenancy?: boolean;
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

/** What someone typed as their organization, as a tenant id: trimmed, lower case. */
export function normalizeTenant(typed: string): string {
  return String(typed ?? "").trim().toLowerCase();
}

/** The page that signs in to one organization through a provider (#117). */
export function credentialPath(provider: string, tenant: string): string {
  return `/login/${encodeURIComponent(provider)}/${encodeURIComponent(normalizeTenant(tenant))}`;
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
