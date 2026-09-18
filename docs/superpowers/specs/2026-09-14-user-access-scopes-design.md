# Design: User Access Scopes (SEC-01)

Record of the design built on 2026-09-14. The contract for app authors and operators
will be documented in [`docs/agent/scopes.md`](../../agent/scopes.md); this document
preserves **why** each piece is designed the way it is.

## Starting point

Stage 2 of the ddcore feature roadmap introduces broader administrative application support,
prioritizing authorization infrastructure that subsequent output and sharing capabilities
rely upon.

Historically, business segregation (isolating companies, branches, business units, or customers)
required ad-hoc hooks across controllers (`permissionQuery`, `hasPermission`), manual checks
in endpoints, or bespoke filters in reports.

SEC-01 establishes reusable user access scopes (the generic equivalent of Frappe's *User Permissions*):
1. Replaces repeated company/unit/customer segregation hooks with declarative restrictions.
2. Enforces restrictions strictly across all 8 access surfaces: reads, searches, writes, reports,
   export, files, history/versions, and real-time events (SSE).
3. Guarantees that two users with identical roles (e.g. two System Managers or two Sales Managers)
   assigned different scopes remain strictly isolated.
4. Preserves privileged execution contexts (`Admin` and explicit `c.IgnorePermissions()`).

## Key Decisions

### 1. Standalone Core DocType: `User Permission`
Instead of embedding scope arrays in the `User` document or using out-of-band JSON config files,
scopes are modeled as a first-class standard DocType in the Core module:
`core/doctypes/user_permission/user_permission.doctype.ts` (`tab_user_permission`).

**Fields:**
- `name`: Generated unique identifier (`naming: { hash: true }`).
- `user`: Link -> `User` (`reqd: true`, `inListView: true`, `inStandardFilter: true`).
- `allow`: Data (`label: "DocType"`, `reqd: true`, `inListView: true`, `inStandardFilter: true`).
  The DocType that defines the scope entity (e.g. `"Company"`, `"Branch"`, `"Customer"`).
- `for_value`: Data (`label: "For Value"`, `reqd: true`, `inListView: true`, `searchIndex: true`).
  The identifier/name of the specific allowed record.
- `applicable_for`: Data (`label: "Applicable For"`, `inListView: true`).
  Optional target DocType. If specified, the restriction only applies when querying or mutating
  that specific DocType. If blank/null, applies globally to all DocTypes referencing `allow`.
- `is_default`: Check (`label: "Is Default"`, `default: false`).
  Optional flag to indicate the default value for autofilling forms in the Desk.

**Permissions:**
- Read, write, create, delete, report, export granted exclusively to `System Manager`.
- Ordinary users cannot inspect or modify user permissions.

### 2. In-Memory Resolution and Caching
Scope rules must not incur round-trip queries on every row evaluation or permission check:
- `engine.Ctx` provides `(c *Ctx) UserPermissions() ([]UserPerm, error)`.
- Returns an empty list immediately for `Admin` or when `c.IgnorePermissions()` is true.
- Cached on the context (`c.userPerms`) for the lifetime of the request.
- Cached in `c.E.Cache` under `"user_perms:" + user`.
- Cache is invalidated whenever a `User Permission` document is created, updated, or deleted
  (using controller hooks / doc save lifecycle).

### 3. Query Engine Enforcement (`permissionFilters`)
The engine's `permissionFilters(d *meta.DocType)` in `internal/engine/perm.go` is the central
chokepoint for set-based queries:
- Bypassed for `Admin` and `c.IgnorePermissions()`.
- For each distinct `allow` DocType with active permissions for `c.User`:
  - Filters matching the current DocType `d` (`up.ApplicableFor == "" || up.ApplicableFor == d.Name`).
  - Assembles `allowedValues = []any{...}`.
  - **Case A: Direct Entity Query (`d.Name == allow`)**:
    Adds `db.Filter{Field: "name", Op: "in", Value: allowedValues}`.
  - **Case B: Referencing DocType (`d.Name != allow`)**:
    Inspects `d.Fields` for `Link` fields where `f.OptionsString() == allow`.
    Adds `db.Filter{Field: f.Fieldname, Op: "in", Value: allowedValues}`.
  - **Strict Segregation**:
    Per design choice, records with null/empty values in restricted link fields are excluded.
    A user scoped to Company A cannot see unassigned or blank-company records, preventing
    cross-tenant data leakage.
  - **Dynamic Links**:
    For `Dynamic Link` fields where the option points to another field (e.g. `party_type` and `party_name`),
    if `party_type == allow`, the query builder applies `party_name IN allowedValues`.

Because `GetList` is the common foundation:
- `Count`: Uses `GetList` with `count(*)`, inheriting scope filters.
- `LinkSearch`: Calls `GetList`, so link autocomplete menus only suggest permitted records.
- `Export`: Paginates using keyset queries over `GetList`, completely preventing data leaks in exports.
- `Reports`: App reports calling `ddcore.db.getList` or `Select` with permissions automatically inherit scopes.

### 4. Document-Level Enforcement (`HasPermission`)
Set-based query filtering prevents finding or listing out-of-scope records, but direct reads
and mutations must be validated per record:
- New engine function `(c *Ctx) checkUserPermissions(d *meta.DocType, doc Doc) (bool, error)`:
  - Bypasses `Admin` and `c.IgnorePermissions()`.
  - When `doc != nil`:
    - Checks `doc.Name()` if `d.Name == allow`.
    - Checks all `Link` fields pointing to `allow`.
    - Checks child tables (`Table` fields) containing rows linking to `allow`.
- Wired into `HasPermission(doctype, ptype string, doc Doc)`:
  - If role-based permissions allow access, `checkUserPermissions` must also pass.
- Wired into operations in `internal/engine/doc.go`:
  - `GetDoc`: Returns `403 PermissionError` if `HasPermission(doctype, "read", doc)` fails.
  - `Insert`: Checks `HasPermission(d.Name, "create", doc)` -> rejects inserting documents into an out-of-scope company.
  - `Update`: Checks `HasPermission(d.Name, ptype, before)` AND `HasPermission(d.Name, ptype, doc)` ->
    prevents reading out-of-scope documents and prevents mutating a document's scope field to an unauthorized value.
  - `Delete`: Checks `HasPermission(doctype, "delete", doc)` -> rejects deleting out-of-scope documents.
  - `Submit`, `Cancel`, `Amend`: Follow `HasPermission` rules.

### 5. Cross-Cutting Access Channels

#### Files (`CanReadFile`)
- In `internal/engine/files.go`, `CanReadFile` checks permission against the attached document
  via `c.GetDoc(attached_to_doctype, attached_to_name)`.
- If the attached document belongs to an out-of-scope company, `GetDoc` returns a permission error,
  and `CanReadFile` evaluates to `false`.
- The historical check `c.HasRole("System Manager")` is refined so that non-Admin users
  with active `User Permission` cannot bypass scope restrictions on attached files.

#### History / Versions (`Version` DocType)
- In `internal/api/api.go`, `requireDocRead(c, doctype, name)` calls `c.GetDoc(doctype, name)`.
- If a user cannot read the target document due to scope restrictions, requests to
  `/api/versions/:doctype/:name` and `/api/comments/:doctype/:name` fail with `403 PermissionError`.

#### Real-time Events (SSE `/api/events`)
- In `internal/api/api.go`, `eventAuthorizer(ctx, user)` is updated:
  - For generic doctype events (`name == ""`), evaluates doctype-level read permission.
  - For document-specific events (`name != ""`), evaluates `c.GetDoc(doctype, name)`.
- Guarantees that document modifications, assignments, and comments occurring in Company B are never
  pushed to users scoped to Company A.

#### Administrative Audit Trail
- Scope administration actions emit unified audit events into `tab_audit_event`:
  - `permission.scope_grant`: when a `User Permission` is created.
  - `permission.scope_revoke`: when a `User Permission` is deleted.
- Denied attempts on out-of-scope documents trigger `c.AuditDenied` where appropriate.

## Verification Strategy

Automated tests in `internal/engine/sec01_test.go` and `internal/api/sec01_test.go`:
1. Two mock segregation entities (`Company` A and B).
2. Two identical users with `System Manager` and custom roles (`user_a` and `user_b`).
3. User permissions created for `user_a` (Company A) and `user_b` (Company B).
4. Assert strict isolation across all 8 surfaces:
   - Reads (`GetDoc`)
   - Lists (`GetList`, `Count`)
   - Searches (`LinkSearch`)
   - Writes (`Insert`, `Update`, `Delete`)
   - Export (`Export`)
   - Reports (simulated report query)
   - Files (`CanReadFile`)
   - History (`requireDocRead` for versions)
   - Events (`Hub.Publish` filtering)
5. Assert that `Admin` and `ignorePermissions: true` have full, unrestricted access.
