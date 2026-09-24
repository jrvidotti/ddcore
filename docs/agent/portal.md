# Portals

A portal is a self-service view of a few DocTypes for people who are not desk users:
employees updating their own details, customers following their orders, suppliers sending
documents. An app declares it in a file. The framework serves it at `/portal`, signs its
users in, keeps them out of the desk, and limits every read and write they make to their
own records.

Signed-in portals only. There are no anonymous Web Forms and no self-registration: every
portal user is invited (see [Inviting portal users](#inviting-portal-users)).

## The model

- **A Website User** (`User.user_type = "Website User"`) reaches the portals and nothing
  else. Signing in sends them to `/portal`, and every `/api` route that is not the portal's
  answers 403, including a whitelisted method not marked `portal: true`. The roles a
  Website User holds decide which portals they reach; those roles open no desk screen.
- **A portal's pages are a Website User's only permission.** They hold no role grant on
  any DocType: every read, list, count, insert, save, attachment and file download they
  make is judged against the pages, inside the engine's permission check. The portal's
  endpoints are therefore not the only guard. A `portal: true` method calling
  `ddcore.getList` sees the same rows the portal shows.
- **"Their own records"** is an *identity* plus a *match*. The identity is the row that
  stands for the signed-in person, such as the Employee whose `user` is them. Each page
  matches its DocType's fields to that row. The identity is read on every request, so
  linking or unlinking a user takes effect immediately and there is nothing to keep in sync.

## Declaring a portal

`portal/<name>.portal.ts`:

```ts
import { definePortal } from "@ddcore/sdk";

export default definePortal({
  name: "Employee",                 // URL: /portal/employee
  title: "Employee Portal",
  roles: ["Employee"],              // who reaches it
  identity: { doctype: "Employee", userField: "user", filters: { status: "Active" } },
  pages: [
    {
      name: "my-data", label: "My data", kind: "record",
      doctype: "Person", match: { id: "person" },      // Person.id = my Employee.person
      fields: ["person_name", "email", "phone", "address_line", "city"],
      actions: [{ label: "Request a change", page: "requests", new: true }],
    },
    {
      name: "documents", label: "My documents",
      doctype: "Employee Document", match: { employee: "id" },   // employee = my Employee.id
      fields: ["document_type", "file", "expiry_date"],
      listFields: ["document_type", "expiry_date"],
      editable: ["document_type", "file", "expiry_date"],
      create: true,
    },
  ],
});
```

| Key | Meaning |
| --- | --- |
| `name` | Unique across apps. Its URL segment is the name in lowercase with dashes. |
| `title` | Shown to the user; a catalogue key. |
| `roles` | Required. The roles that reach the portal. |
| `identity.doctype`, `identity.userField` | The rows whose `userField`, a Link to User, is the signed-in user. For a portal over the user themselves, use `{ doctype: "User", userField: "id" }`. |
| `identity.filters` | Equality filters the identity must also meet, e.g. `{ status: "Active" }`. |
| `pages[].name` | Lowercase letters, digits and dashes; the page's URL segment. |
| `pages[].kind` | `list` (default) lists the user's rows; `record` opens their single row directly. |
| `pages[].match` | **Required.** Page field to identity field. Every pair must hold against one identity row. `id` is allowed on either side. |
| `pages[].fields` | The fields shown. Level 0 only; never a Table, Table MultiSelect, Password or Vault field. Fields the page does not name are never sent. |
| `pages[].listFields` | The list's columns. Defaults to the first four `fields`. |
| `pages[].editable` | The fields the user may type into: a subset of `fields`, never a match field, never read-only. |
| `pages[].create`, `pages[].write` | Whether the page creates documents, and whether it saves existing ones. Read is always granted. |
| `pages[].defaultsMethod` | A `whitelisted(fn, { portal: true })` method whose result prefills a new document, limited to `editable`. |
| `pages[].actions` | Buttons to other pages of the portal; `new: true` opens the target's creation form. |

The load refuses a portal that names a missing DocType or field, a restricted (`permlevel`
above 0) or Table field, an editable field outside `fields`, an editable match field, a
page with `create` or `write` and nothing editable, a record page that creates, or a
`defaultsMethod` without `portal: true`.

`title`, each page's `label` and `description`, and each action's `label` are collected by
`ddcore i18n extract`.

## What a page grants

| Operation | Granted when |
| --- | --- |
| read, list, count, file download | the document matches some page over its DocType (`fields` then limits what is sent and which attached files may be read) |
| create | a page with `create: true`. The match fields are set from the identity, and a body that names them is refused |
| write | a page with `write: true`, on a matching document, for its `editable` fields only |
| delete, submit, cancel, amend, share, export, import, report | never |

After the page's grant, the rest of the permission check still applies: the workflow
state's `allowEdit`, the controller's `hasPermission` and `permissionQuery`, and User
Permission scopes. A document in a workflow state whose `allowEdit` names an HR role cannot be
saved from the portal, which is usually what you want once it is under review. Field levels
give a Website User level 0 only.

Internal lookups stay elevated as they always were: link validation, `fetchFrom`, link
titles and `ddcore.db.getValue`/`getAll` in a controller. A portal insert can therefore
fetch from, and link to, DocTypes no page shows.

## Endpoints

All of them run in portal mode for anyone, including a desk user previewing the portal, and
answer only for portals the user's roles reach.

| Request | Answer |
| --- | --- |
| `GET /api/portal` | The portals and their pages (also in `/api/boot` as `portals`). |
| `GET /api/portal/{portal}/{page}` | The page and its translated fields, non-editable ones marked `readOnly`. |
| `GET /api/portal/{portal}/{page}/list?start&limit` | `{ rows, titles, more }`: the user's rows on that page. |
| `GET /api/portal/{portal}/{page}/doc[/{id}]` | One document, projected to the page's fields. Without an id on a `record` page, the user's own. |
| `GET /api/portal/{portal}/{page}/new` | The `defaultsMethod` values, limited to `editable`. |
| `POST /api/portal/{portal}/{page}/doc` | Create. It starts from the `defaultsMethod` values; any key outside `editable` is a 417. |
| `PUT /api/portal/{portal}/{page}/doc/{id}` | Save the `editable` keys. Send `modified` to be refused (409) if someone saved in between. |
| `GET /api/portal/{portal}/{page}/search/{field}?q=` | Choices for an editable Link field: `[{ id, title }]`, read with permissions ignored. That is why only an editable Link can be searched. |

Also open to a Website User: `/api/login`, `/api/logout`, `/api/auth/*`, `/api/boot`
(portals, language and site only), `/api/translations`, `/api/upload`, `/api/file-info`,
`/files/*`, `/private/files/*`, and `/api/method/<path>` for methods whitelisted with
`portal: true`. The core's self-service methods (profile, password, sessions, language) are
marked, so the portal's account screen works. API keys are not.

## Files

A Website User's upload must name the page's DocType and an `Attach` or `Attach Image`
field that some page with `create` or `write` makes editable, and it is always stored
private. On a new document the file is uploaded detached. The save that names it
attaches it to the document (this is true for every user, not only in portals: see
[storage](storage.md)). From then on it follows the document: the HR user reviewing
the document can read it, and another employee cannot.

A Website User downloads a file only when it is attached, through a field some page shows,
to a document they can read.

## Abuse controls

A Website User's portal creates and saves, and their uploads, are counted per user over a
sliding hour. Past the limit the answer is 429 with `Retry-After`. Uploads are also capped
in size. `ddcore.json`:

```json
"portal": { "writesPerHour": 60, "uploadsPerHour": 30, "maxUploadMB": 10 }
```

Zero or absent takes those defaults. The count lives in the server process: several
processes each allow the whole budget.

## Inviting portal users

`ddcore.users.invite` creates an account with no password and mails the invitation. Its
link comes back in the result when the site does not really deliver mail.

```ts
export const inviteEmployee = whitelisted((args: { employee: string }) => {
  const emp = ddcore.getDoc("Employee", args.employee);
  const res = ddcore.users.invite({ email: emp.email, fullName: emp.employee_name, roles: ["Employee"], userType: "Website User" });
  ddcore.db.setValue("Employee", emp.id, "user", res.user);
  return res;
}, { roles: ["HR Manager"] });
```

A System Manager may invite anyone. Any other caller, which here means the app's own method
reached by a narrower role, may only create a **Website User** and never give `System Manager`,
`Admin`, `All` or `Guest`. That is safe because a Website User reaches nothing but portals.
Who may call the method is the app's whitelist to decide. `ddcore.users.resendInvite(user)`
follows the same rule. Both write the `account.invite` / `account.resend_invite` audit events.

## The desk side

- `/portal` has its own layout: the site name, the pages of every portal the user
  reaches, and an account menu (profile, sign out). None of the desk's shell, search,
  notifications or event stream is loaded.
- A Website User who opens any other desk path is sent to `/portal`. A desk user whose roles
  reach a portal finds **My portal** in the account menu. Inside it they are judged like
  anyone else, by the pages and their identity row: an HR analyst who is also an employee
  sees their own records, and one with no identity row sees nothing.
- Forms are built from the page's fields, and Link fields search through the page's own
  endpoint. Form scripts (`*.form.ts`) do not run in the portal. Global scripts do, when they
  are listed under `portal.include` (below).

## Client scripts on portal pages

`defineApp`'s `portal.include` lists client scripts, relative to the app dir, that load on
every portal page. They are bundled on their own, apart from `desk.include`:

```ts
defineApp({
  desk: { include: ["client/masks.ts"] },
  portal: { include: ["client/masks.ts", "client/cep.ts"] },
});
```

- A portal input carries `data-fieldname` and `data-fieldtype`, like a desk one, so a script that
  works by event delegation on `document` behaves the same in both places.
- To set a field from a script, write the value and fire `input`. The form state follows the
  event, not the DOM:
  `el.value = v; el.dispatchEvent(new Event("input", { bubbles: true }))`.
- A Website User is refused the desk API. A script that calls the server calls a service
  whitelisted with `{ portal: true }`.
- A Website User loads these scripts at sign-in. A desk user loads them on first entering
  **My portal**, and they stay loaded when that user goes back to the desk. A script listed in
  both blocks then runs twice there, so it should guard its own setup
  (`if (window.__myMasks) return; window.__myMasks = true;`).

## Not here yet

- Anonymous Web Forms (a job application, a contact form) and self-registration.
- Table (child) fields on a page, and delete, submit or workflow actions from the portal.
  A workflow's state is shown, not moved.
- Notifications in the portal, and a portal inbox.
- Per-page roles and per-page filters: a page reaches every user of its portal.
- Rate limits are per process (see PRD-08), and in memory.
- `dependsOn` and the other field conditions are dropped on a portal page, because they may
  name fields the page does not carry.
