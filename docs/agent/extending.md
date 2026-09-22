# Extending another app's DocTypes

An app adds fields to, and changes properties of, a DocType another app owns — without forking it.
This is where a Frappe site's Custom Fields and Property Setters land: in versioned TypeScript,
not in a table.

```ts
// apps/billing/extensions/lead.extend.ts
import { extendDoctype } from "@ddcore/sdk";

export default extendDoctype("Lead", {          // Lead belongs to the `crm` app
  fields: [
    { fieldname: "invoice_no", fieldtype: "Data", label: "Invoice no", insertAfter: "title" },
  ],
  set: {                                        // property setters, per field
    status: { reqd: true, inListView: true },
  },
  doctype: { trackChanges: true },              // property setters on the DocType itself
  permissions: [{ role: "Accountant", read: true, report: true }],
  hasPermission(doc, ptype) { if (ptype === "delete" && doc.invoice_no) return false; },
  permissionQuery(user) { return { owner: user }; },
});
```

`ddcore.app.ts` must declare the host: `requires: ["crm"]`. Core is implicit — it always loads
first, and the core releases the app supports go in `ddcore:` (see the compatibility contract in
`conventions`). Files go in `extensions/<snake>.extend.ts`; a form script for the same DocType sits beside
it as `extensions/<snake>.form.ts`.

The merge happens once, in the engine, before the meta is validated. From then on there is one
DocType: the column is created by `migrate`, the field appears in `.ddcore/types.d.ts`, in the API,
in the list and in the form, and `reqd` is enforced on the server. Nothing downstream knows the
field came from elsewhere.

## What refuses to load

An extension is not a patch applied in whatever order the apps happen to install in. The effective
meta has to be the same on every machine, so a clash is an error at load — `dev` keeps serving the
last good state and `migrate` stops:

- the DocType does not exist, or the app owns it (edit the DocType instead);
- the host app is not in `requires`;
- the fieldname is already taken, by the host or by another extension;
- two apps set the same property on the same field, or the same DocType property;
- an extension grants a role the host already grants at the same `permlevel` — an extension only *adds* roles. Granting a field level (`permlevel: 1`) to a role the host grants at level 0 is an addition; setting `permlevel` on a host field through `set` is allowed too. See `field-permissions`.

Not overridable at all: `fieldname` and `fieldtype`, a Link's or a Table's `options` (they name
the target), and `idGeneration`, `isChild`, `isSingle`, `submittable` and `module` on the DocType. Those
decide what the document *is*, and stay with the app that declares it. A Select's `options` are
text and may be replaced.

## Scripts

Behaviour needs no extension — it already crosses apps:

- **Server**: `docEvents` in `defineApp` attaches lifecycle hooks to any DocType, or to `"*"`.
  See `controller-api`. `defineController` stays with the owner: a second app's controller for the
  same DocType is refused, because it would replace the first one whole.
- **Desk**: `extensions/<snake>.form.ts` is loaded alongside the owner's `<snake>.form.ts` — form
  handlers accumulate, they do not replace one another. A `client/*.ts` in `desk.include` can call
  `defineForm` for any DocType too. `defineListView` is the exception: one per DocType, last
  registration wins.

`hasPermission` and `permissionQuery` above are the chained pair: every denial counts (any `false`
denies, `undefined` is no opinion) and every filter set is AND-ed. An extension can restrict what
the host allows, never widen it.

## Translations

A label written in an extension is a key in **that** app's catalogue, not the host's — including a
label it overrides. `ddcore i18n extract` files it there, and `--check` asks the app that wrote the
string for the translation. See `i18n`.

## Removing one

Deleting the extension removes the field from the meta, but `migrate` does not drop the column
unless it is run with `--prune`; until then the data is still there and reads still return it.
That is the same rule any removed field follows.

`extendDoctype` stretches a DocType another app owns. When what stands in the way is the core
itself — a capability ddcore does not have at all — the route is upstream; see `feature-requests`.
