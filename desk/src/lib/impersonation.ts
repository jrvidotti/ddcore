import { api } from "$lib/api";
import { seg } from "$lib/routes";
import { doctypeWsPrefix } from "$lib/components/sidebar-workspace";

// Impersonating changes who the whole page is signed in as, so both ways are
// a full load, like entering a tenant: nothing of the other user's boot,
// notifications or event stream survives the switch.

/** Works as user from the next request on, and opens where that user lands. */
export async function impersonate(user: string) {
  const r = await api.impersonate(user);
  location.href = r?.home || "/app";
}

/** Returns the session to the administrator, on the record of the user they were working as. */
export async function stopImpersonating() {
  const r = await api.stopImpersonating();
  location.href = r?.impersonated ? `${doctypeWsPrefix("User")}/${seg("User")}/${encodeURIComponent(r.impersonated)}` : "/app";
}
