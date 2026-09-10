// Shim: `@ddcore/desk-sdk` inside app client bundles resolves to the runtime
// the desk exposes on window.__ddcoreDesk.
const d = globalThis.__ddcoreDesk;
if (!d) throw new Error("@ddcore/desk-sdk: o desk ainda não carregou");
export const defineForm = d.defineForm;
export const defineListView = d.defineListView;
export const ddcore = d.ddcore;
export const _ = d.ddcore._;
export default d;
