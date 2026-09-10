// Shim: `@cerne/desk-sdk` inside app client bundles resolves to the runtime
// the desk exposes on window.__cerneDesk.
const d = globalThis.__cerneDesk;
if (!d) throw new Error("@cerne/desk-sdk: o desk ainda não carregou");
export const defineForm = d.defineForm;
export const defineListView = d.defineListView;
export const cerne = d.cerne;
export const _ = d.cerne._;
export default d;
