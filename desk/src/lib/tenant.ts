// Tenancy in the desk: where the person is working, and — for the operator —
// the tenants there are to enter. The server decides both; see /api/boot.

export interface TenantBoot {
  /** The tenant the session works in; "" is the platform space. */
  id: string;
  title: string;
  /** An operator: may enter a tenant, and leave it again. */
  platform: boolean;
  tenants?: { id: string; title: string; enabled: boolean }[];
}

export interface TenantChoice {
  /** "" returns to the platform space. */
  id: string;
  label: string;
  current: boolean;
}

/** What to call the space the person is in; "" when the site has no tenancy. */
export function tenantLabel(t: TenantBoot | null | undefined, platformLabel: string): string {
  if (!t) return "";
  if (t.id === "") return t.platform ? platformLabel : "";
  return t.title || t.id;
}

/**
 * The places an operator can go: the platform space, then every enabled
 * tenant. Nobody else has anywhere to go, and a disabled tenant admits no one.
 */
export function tenantChoices(t: TenantBoot | null | undefined, platformLabel: string): TenantChoice[] {
  if (!t?.platform) return [];
  const out: TenantChoice[] = [{ id: "", label: platformLabel, current: t.id === "" }];
  for (const x of t.tenants || []) {
    if (!x.enabled && x.id !== t.id) continue;
    out.push({ id: x.id, label: x.title || x.id, current: x.id === t.id });
  }
  return out;
}

/**
 * The tenant a desk URL names (`?tenant=`), so a link copied from the address
 * bar opens in the space it was copied from. Null in the platform space and on
 * a site without tenancy: a link there names no tenant.
 */
export function tenantParam(t: TenantBoot | null | undefined): string | null {
  return t?.id ? t.id : null;
}

/**
 * What a link's `?tenant=` asks of the person following it: nothing ("ok"),
 * to enter that tenant first ("enter", an operator only), or nothing they can
 * do ("foreign", the link is for a space they cannot work in).
 */
export function tenantLinkState(t: TenantBoot | null | undefined, param: string | null): "ok" | "enter" | "foreign" {
  if (!t || !param || param === t.id) return "ok";
  if (t.platform && t.tenants?.some((x) => x.id === param && x.enabled)) return "enter";
  return "foreign";
}

/**
 * The same desk page in the platform space: the URL without its `?tenant=`,
 * for an operator leaving a tenant to change what every tenant shares.
 */
export function platformHref(url: URL): string {
  const u = new URL(url);
  u.searchParams.delete("tenant");
  return u.pathname + u.search + u.hash;
}
