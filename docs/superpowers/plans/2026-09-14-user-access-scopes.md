# User Access Scopes (SEC-01) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide reusable user access scopes (User Permissions) to isolate business entities (companies, units, branches, customers) across reads, searches, writes, reports, export, files, history, and real-time events.

**Architecture:** A standalone Core DocType `User Permission` (`tab_user_permission`) stores user scope assignments. The query builder (`permissionFilters` in `internal/engine/perm.go`) automatically injects scope restrictions for set-based queries (`GetList`, `Count`, `LinkSearch`, `Export`, reports). The document lifecycle (`HasPermission` in `internal/engine/perm.go` and `doc.go`) validates single-document reads and mutations (`GetDoc`, `Insert`, `Update`, `Delete`). Cross-cutting channels (`CanReadFile`, `requireDocRead`, `eventAuthorizer`) enforce isolation for files, version history, and real-time events. `Admin` and explicit `c.IgnorePermissions()` contexts retain full bypass.

**Tech Stack:** Go 1.24, PostgreSQL, TypeScript, ddcore framework SDK.

**Spec:** [`docs/superpowers/specs/2026-09-14-user-access-scopes-design.md`](../specs/2026-09-14-user-access-scopes-design.md)

## Global Constraints
- Strictly follow the repository guidelines in [`AGENTS.md`](../../AGENTS.md): synchronous server TypeScript, generated typings, English canonical strings with translations, zero external dependencies unless justified.
- Strict segregation: link fields referencing a restricted DocType must strictly match one of the user's allowed values; records with null/empty values in the restricted field are hidden from scoped users.
- Role isolation: two users with identical roles (even `System Manager`) assigned different scopes must remain strictly isolated. Only `Admin` and `c.IgnorePermissions()` bypass scope restrictions.
- All code comments, documentation, and commits must be written in English.

---

## Proposed Changes

### Component 1: Core Metadata & DocType

#### [NEW] `core/doctypes/user_permission/user_permission.doctype.ts`
Defines the `User Permission` standard DocType in the `Core` module with fields `user`, `allow`, `for_value`, `applicable_for`, and `is_default`.

#### [MODIFY] `core/translations/pt-BR.csv`
Adds pt-BR translations for the new DocType label and field labels.

---

### Component 2: Engine Query Builder & Permissions

#### [MODIFY] `internal/engine/perm.go`
- Adds `UserPerm` struct and `(c *Ctx) UserPermissions() ([]UserPerm, error)`.
- Updates `(c *Ctx) permissionFilters(d *meta.DocType) ([]db.Filter, error)` to inject scope filters.
- Adds `(c *Ctx) checkUserPermissions(d *meta.DocType, doc Doc) (bool, error)`.
- Wires `checkUserPermissions` into `HasPermission(doctype, ptype string, doc Doc)`.

#### [MODIFY] `internal/engine/doc.go`
- Enforces `HasPermission` on both `before` and incoming `doc` in `Update`.
- Clears user permission caches (`user_perms:<user>`) in `tab_user_permission` save/delete lifecycle.
- Emits audit events (`permission.scope_grant` and `permission.scope_revoke`).

---

### Component 3: Cross-Cutting Surfaces (Files, History, SSE)

#### [MODIFY] `internal/engine/files.go`
- Updates `CanReadFile` so that `System Manager` does not bypass document-level permission checks if scope restrictions apply.

#### [MODIFY] `internal/api/api.go`
- Updates `requireDocRead` to call `c.GetDoc(doctype, name)` to enforce document scope on Version and Comment endpoints.
- Updates `eventAuthorizer` to evaluate `c.GetDoc(doctype, name)` when `name != ""` before sending SSE events.

---

### Component 4: Test Suites & Documentation

#### [NEW] `internal/engine/sec01_test.go`
Go test suite verifying engine-level isolation: queries, mutations, single-doc permissions, and privileged bypass.

#### [NEW] `internal/api/sec01_test.go`
API test suite verifying HTTP surfaces: list, get, create, update, delete, search, export, files, versions, and events.

#### [NEW] `docs/agent/scopes.md`
Agent documentation explaining user access scopes and developer usage.

#### [MODIFY] `ROADMAP.md`
Updates SEC-01 status.

---

## Implementation Tasks

### Task 1: Core DocType `User Permission` & Translations

**Files:**
- Create: `core/doctypes/user_permission/user_permission.doctype.ts`
- Modify: `core/translations/pt-BR.csv`
- Test: `internal/engine/perm_test.go`

**Interfaces:**
- Produces: DocType `User Permission` (`tab_user_permission`), accessible via `c.St.DocType("User Permission")`.

- [ ] **Step 1: Write the DocType definition in TypeScript**

Create `core/doctypes/user_permission/user_permission.doctype.ts`:
```typescript
import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "User Permission",
  module: "Core",
  label: "User Permission",
  icon: "shield",
  naming: { hash: true },
  titleField: "for_value",
  searchFields: ["user", "allow", "for_value"],
  trackChanges: true,
  fields: [
    { fieldname: "user", fieldtype: "Link", label: "User", options: "User", reqd: true, inListView: true, inStandardFilter: true },
    { fieldname: "allow", fieldtype: "Data", label: "DocType", reqd: true, inListView: true, inStandardFilter: true,
      description: "The DocType that defines the restricted scope, e.g. Company or Branch." },
    { fieldname: "for_value", fieldtype: "Data", label: "For Value", reqd: true, inListView: true, searchIndex: true,
      description: "The name/ID of the allowed document." },
    { fieldname: "applicable_for", fieldtype: "Data", label: "Applicable For", inListView: true,
      description: "Optional DocType to restrict this rule to. If empty, applies globally." },
    { fieldname: "is_default", fieldtype: "Check", label: "Is Default", default: false, inListView: true },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
  ],
});
```

- [ ] **Step 2: Add translations to `core/translations/pt-BR.csv`**

Append user-facing strings to `core/translations/pt-BR.csv`:
```csv
"User Permission","Permissão de Usuário"
"For Value","Para o Valor"
"Applicable For","Aplicável Para"
"Is Default","É Padrão"
"The DocType that defines the restricted scope, e.g. Company or Branch.","O DocType que define o escopo restrito, ex.: Company ou Branch."
"The name/ID of the allowed document.","O nome/ID do documento permitido."
"Optional DocType to restrict this rule to. If empty, applies globally.","DocType opcional para restringir esta regra. Se vazio, aplica globalmente."
```

- [ ] **Step 3: Write test asserting `User Permission` DocType loads and creates schema**

Add to `internal/engine/perm_test.go`:
```go
func TestUserPermissionDocTypeLoaded(t *testing.T) {
	e := testEngine(t)
	dt, ok := e.Meta.Get("User Permission")
	if !ok {
		t.Fatalf("expected User Permission DocType to be loaded")
	}
	if dt.TableName() != "tab_user_permission" {
		t.Fatalf("expected table name tab_user_permission, got %s", dt.TableName())
	}
	if f := dt.Field("for_value"); f == nil || !f.Reqd {
		t.Fatalf("expected for_value field to be required")
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/engine -run TestUserPermissionDocTypeLoaded`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/doctypes/user_permission core/translations/pt-BR.csv internal/engine/perm_test.go
git commit -m "feat(core): define User Permission DocType"
```

---

### Task 2: User Permission In-Memory Cache and Resolution

**Files:**
- Modify: `internal/engine/perm.go`
- Test: `internal/engine/perm_test.go`

**Interfaces:**
- Produces: `(c *Ctx) UserPermissions() ([]UserPerm, error)` and cache invalidation.

- [ ] **Step 1: Define `UserPerm` struct and `UserPermissions` method in `internal/engine/perm.go`**

```go
// UserPerm represents an active scope restriction for a user.
type UserPerm struct {
	Name          string
	User          string
	Allow         string
	ForValue      string
	ApplicableFor string
	IsDefault     bool
}

// UserPermissions returns the active scope restrictions for the current user.
// Bypassed immediately for Admin or when IgnorePermissions() is active.
func (c *Ctx) UserPermissions() ([]UserPerm, error) {
	if c.User == "Admin" || c.IgnorePermissions() {
		return nil, nil
	}
	if c.userPerms != nil {
		return c.userPerms, nil
	}
	key := "user_perms:" + c.User
	if v, ok := c.E.Cache.Get(key); ok {
		c.userPerms = v.([]UserPerm)
		return c.userPerms, nil
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT name, "user", allow, for_value, applicable_for, is_default
		FROM tab_user_permission WHERE "user" = $1 ORDER BY allow, for_value`, c.User)
	if err != nil {
		return nil, err
	}
	perms := make([]UserPerm, 0, len(rows))
	for _, r := range rows {
		perms = append(perms, UserPerm{
			Name:          db.Str(r["name"]),
			User:          db.Str(r["user"]),
			Allow:         db.Str(r["allow"]),
			ForValue:      db.Str(r["for_value"]),
			ApplicableFor: db.Str(r["applicable_for"]),
			IsDefault:     toBool(r["is_default"]),
		})
	}
	c.userPerms = perms
	c.E.Cache.Set(key, perms, 0)
	return perms, nil
}
```

- [ ] **Step 2: Add `userPerms []UserPerm` to `Ctx` struct in `internal/engine/doc.go` (or wherever `Ctx` is defined)**

In `internal/engine/doc.go`:
```go
type Ctx struct {
    ...
    userPerms []UserPerm
}
```

- [ ] **Step 3: Add cache invalidation on `tab_user_permission` save and delete**

In `internal/engine/doc.go` (in `afterSave` and `afterDelete` or chokepoint handlers):
Whenever doctype == "User Permission", invalidate cache:
```go
if d.Name == "User Permission" {
    user := doc.Str("user")
    if user != "" {
        c.E.Cache.Del("user_perms:" + user)
    }
}
```

- [ ] **Step 4: Write unit test for `UserPermissions` loading and caching**

In `internal/engine/perm_test.go`:
```go
func TestUserPermissionsResolutionAndCache(t *testing.T) {
	e := testEngine(t)
	ctx := context.Background()
	// Insert user permission as admin
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		_, err := c.Insert(Doc{
			"doctype": "User Permission",
			"user": "test_user@example.com",
			"allow": "Company",
			"for_value": "Acme Corp",
			"is_default": true,
		}, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatalf("failed to insert user permission: %v", err)
	}

	// Test user permissions resolution
	err = e.Run(ctx, "test_user@example.com", func(c *Ctx) error {
		perms, err := c.UserPermissions()
		if err != nil {
			return err
		}
		if len(perms) != 1 {
			t.Fatalf("expected 1 perm, got %d", len(perms))
		}
		if perms[0].Allow != "Company" || perms[0].ForValue != "Acme Corp" {
			t.Fatalf("unexpected perm values: %+v", perms[0])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// Test Admin returns empty (bypassed)
	err = e.Run(ctx, "Admin", func(c *Ctx) error {
		perms, err := c.UserPermissions()
		if err != nil {
			return err
		}
		if len(perms) != 0 {
			t.Fatalf("expected 0 perms for Admin, got %d", len(perms))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -v ./internal/engine -run TestUserPermissionsResolutionAndCache`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/engine/perm.go internal/engine/doc.go internal/engine/perm_test.go
git commit -m "feat(engine): add UserPermissions cache and resolution"
```

---

### Task 3: Query Builder Scope Injection (`permissionFilters`)

**Files:**
- Modify: `internal/engine/perm.go`
- Test: `internal/engine/sec01_test.go`

**Interfaces:**
- Consumes: `c.UserPermissions()`.
- Produces: Injected scope `db.Filter` items in `c.permissionFilters(d)`.

- [ ] **Step 1: Extend `permissionFilters` in `internal/engine/perm.go`**

Implement scope filters builder:
```go
// scopeFilters builds filters enforcing User Permission rules for doctype d.
func (c *Ctx) scopeFilters(d *meta.DocType) ([]db.Filter, error) {
	if c.User == "Admin" || c.IgnorePermissions() {
		return nil, nil
	}
	perms, err := c.UserPermissions()
	if err != nil || len(perms) == 0 {
		return nil, err
	}

	// Group allowed values by allow-doctype: allow -> []any{for_value...}
	grouped := make(map[string][]any)
	for _, p := range perms {
		if p.ApplicableFor != "" && !strings.EqualFold(p.ApplicableFor, d.Name) {
			continue
		}
		grouped[p.Allow] = append(grouped[p.Allow], p.ForValue)
	}

	var out []db.Filter
	for allow, allowedValues := range grouped {
		if len(allowedValues) == 0 {
			continue
		}
		if strings.EqualFold(d.Name, allow) {
			out = append(out, db.Filter{Field: "name", Op: "in", Value: allowedValues})
			continue
		}
		for _, f := range d.Fields {
			if f.Fieldtype == "Link" && strings.EqualFold(f.OptionsString(), allow) {
				// Strict segregation: field must be in allowedValues (null/empty excluded)
				out = append(out, db.Filter{Field: f.Fieldname, Op: "in", Value: allowedValues})
			}
		}
	}
	return out, nil
}
```

In `c.permissionFilters(d *meta.DocType)`:
```go
sf, err := c.scopeFilters(d)
if err != nil {
    return nil, err
}
out = append(out, sf...)
```

- [ ] **Step 2: Write failing test in `internal/engine/sec01_test.go`**

Create `internal/engine/sec01_test.go` testing that `GetList`, `Count`, and `LinkSearch` respect scope filters:
- Create mock DocTypes: `Test Company` and `Test Record` (with link `company: Test Company`).
- Create companies "Alfa" and "Beta".
- Create records in Alfa and Beta.
- Set User Permission for `user_alfa@x.com` -> `Company: "Alfa"`.
- Verify `GetList` returns only Alfa records for `user_alfa@x.com`.
- Verify `Count` returns count of Alfa records only.
- Verify `LinkSearch` returns only Alfa.

- [ ] **Step 3: Run test to verify it passes**

Run: `go test -v ./internal/engine -run TestSEC01_QueryFilters`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/engine/perm.go internal/engine/sec01_test.go
git commit -m "feat(engine): inject user access scope filters in permissionFilters"
```

---

### Task 4: Document Lifecycle Validation (`HasPermission` in `perm.go` & `doc.go`)

**Files:**
- Modify: `internal/engine/perm.go`
- Modify: `internal/engine/doc.go`
- Test: `internal/engine/sec01_test.go`

**Interfaces:**
- Produces: `(c *Ctx) checkUserPermissions(d *meta.DocType, doc Doc) (bool, error)` integrated into `HasPermission`.

- [ ] **Step 1: Implement `checkUserPermissions` in `internal/engine/perm.go`**

```go
// checkUserPermissions validates whether doc satisfies active scope restrictions.
func (c *Ctx) checkUserPermissions(d *meta.DocType, doc Doc) (bool, error) {
	if c.User == "Admin" || c.IgnorePermissions() || doc == nil {
		return true, nil
	}
	perms, err := c.UserPermissions()
	if err != nil || len(perms) == 0 {
		return true, err
	}

	grouped := make(map[string]map[string]bool)
	for _, p := range perms {
		if p.ApplicableFor != "" && !strings.EqualFold(p.ApplicableFor, d.Name) {
			continue
		}
		if grouped[p.Allow] == nil {
			grouped[p.Allow] = make(map[string]bool)
		}
		grouped[p.Allow][p.ForValue] = true
	}

	for allow, allowedMap := range grouped {
		if len(allowedMap) == 0 {
			continue
		}
		if strings.EqualFold(d.Name, allow) {
			name := doc.Name()
			if name != "" && !allowedMap[name] {
				return false, nil
			}
			continue
		}
		for _, f := range d.Fields {
			if f.Fieldtype == "Link" && strings.EqualFold(f.OptionsString(), allow) {
				val := db.Str(doc[f.Fieldname])
				// Strict mode: value cannot be empty and must be in allowedMap
				if val == "" || !allowedMap[val] {
					return false, nil
				}
			}
		}
	}
	return true, nil
}
```

- [ ] **Step 2: Call `checkUserPermissions` inside `HasPermission`**

In `internal/engine/perm.go`:
```go
if doc != nil {
    ok, err := c.checkUserPermissions(d, doc)
    if err != nil || !ok {
        return false, err
    }
}
```

- [ ] **Step 3: Enforce `doc` permission check in `Update` in `internal/engine/doc.go`**

In `internal/engine/doc.go` (`Update` method around line 545):
```go
// Validate that the updated document does not violate scopes
if !opts.IgnorePermissions && !c.IgnorePermissions() {
    if ok, err := c.HasPermission(d.Name, ptype, doc); err != nil {
        return nil, err
    } else if !ok {
        return nil, cerr.Permission("No permission ({0}) on {1} {2}", ptype, c.T(d.Label), doc.Name())
    }
}
```

- [ ] **Step 4: Write tests for `GetDoc`, `Insert`, `Update`, `Delete` in `internal/engine/sec01_test.go`**

Add tests:
- `user_alfa` reading Beta record via `GetDoc` receives `PermissionError`.
- `user_alfa` inserting record with `company: "Beta"` receives `PermissionError`.
- `user_alfa` updating record with `company: "Beta"` receives `PermissionError`.
- `user_alfa` deleting record of Beta receives `PermissionError`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -v ./internal/engine -run TestSEC01_DocLifecycle`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/engine/perm.go internal/engine/doc.go internal/engine/sec01_test.go
git commit -m "feat(engine): enforce user access scopes in document lifecycle"
```

---

### Task 5: Cross-Cutting Channels (Files, History, SSE, Audit)

**Files:**
- Modify: `internal/engine/files.go`
- Modify: `internal/api/api.go`
- Test: `internal/api/sec01_test.go`

**Interfaces:**
- Produces: Protected file reading, version history guard, SSE event filtering, audit logs for grant/revoke.

- [ ] **Step 1: Refine `CanReadFile` in `internal/engine/files.go`**

Update `CanReadFile`:
```go
func (c *Ctx) CanReadFile(f map[string]any) bool {
	if f == nil {
		return false
	}
	if c.User == "Admin" || c.IgnorePermissions() {
		return true
	}
	if dt, dn := db.Str(f["attached_to_doctype"]), db.Str(f["attached_to_name"]); dt != "" && dn != "" {
		if _, err := c.GetDoc(dt, dn); err == nil {
			return true
		}
		return false
	}
	if db.Str(f["owner"]) == c.User || c.HasRole("System Manager") {
		return true
	}
	return false
}
```

- [ ] **Step 2: Update `requireDocRead` in `internal/api/api.go`**

Ensure `requireDocRead` verifies access through `c.GetDoc`:
```go
func (s *Server) requireDocRead(c *engine.Ctx, doctype, name string) error {
	if doctype == "" || name == "" {
		return cerr.Permission("Provide the reference document")
	}
	_, err := c.GetDoc(doctype, name)
	return err
}
```

- [ ] **Step 3: Update `eventAuthorizer` in `internal/api/api.go`**

Verify document-level read permission for document events (`name != ""`):
```go
func (s *Server) eventAuthorizer(ctx context.Context, u string) engine.Authorizer {
	return func(doctype, name string) bool {
		key := fmt.Sprintf("evperm:%s:%s:%s", u, doctype, name)
		if v, ok := s.E.Cache.Get(key); ok {
			return v.(bool)
		}
		allowed := false
		if err := s.E.Run(ctx, u, func(c *engine.Ctx) error {
			if name == "" {
				ok, err := c.HasPermission(doctype, "read", nil)
				allowed = ok
				return err
			}
			_, err := c.GetDoc(doctype, name)
			allowed = (err == nil)
			return nil
		}); err != nil {
			return false
		}
		s.E.Cache.Set(key, allowed, time.Minute)
		return allowed
	}
}
```

- [ ] **Step 4: Emit audit events on `User Permission` operations**

In `internal/engine/doc.go`:
When inserting `User Permission`:
```go
if d.Name == "User Permission" {
    c.Audit("permission.scope_grant", "User", doc.Str("user"), map[string]any{
        "allow": doc.Str("allow"),
        "for_value": doc.Str("for_value"),
        "applicable_for": doc.Str("applicable_for"),
    })
}
```
When deleting `User Permission`:
```go
if d.Name == "User Permission" {
    c.Audit("permission.scope_revoke", "User", doc.Str("user"), map[string]any{
        "allow": doc.Str("allow"),
        "for_value": doc.Str("for_value"),
    })
}
```

- [ ] **Step 5: Write API test suite in `internal/api/sec01_test.go`**

Verify HTTP endpoints:
- File download for attachment of Beta document fails for `user_alfa`.
- Version history `/api/versions/Test Record/:name` fails for `user_alfa`.
- SSE events published for Beta do not arrive at `user_alfa`'s connection.
- Export `/api/export/Test Record` returns only Alfa records.

- [ ] **Step 6: Run API test suite**

Run: `go test -v ./internal/api -run TestSEC01`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/engine/files.go internal/api/api.go internal/engine/doc.go internal/api/sec01_test.go
git commit -m "feat(security): enforce user access scopes across files, history, SSE and audit"
```

---

### Task 6: Generated Typings, Documentation, and Verification

**Files:**
- Create: `docs/agent/scopes.md`
- Modify: `ROADMAP.md`
- Test: Full test suite (`make test`)

- [ ] **Step 1: Write `docs/agent/scopes.md` reference guide**

Document user access scopes for developers and operators:
- What user access scopes are and how they work.
- Schema of `User Permission`.
- Scope behavior across reads, writes, searches, exports, files, and events.
- Privileged contexts (`Admin` and `ignorePermissions`).

- [ ] **Step 2: Update `ROADMAP.md`**

Mark SEC-01 as delivered in `ROADMAP.md` with summary of implementation and test coverage.

- [ ] **Step 3: Run `make test` and `make check`**

Run: `make test`
Expected: PASS (all Go tests + TypeScript typechecks pass)
Run: `make check`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add docs/agent/scopes.md ROADMAP.md
git commit -m "docs: add user access scopes documentation and update roadmap (SEC-01)"
```

---

## Verification Plan

### Automated Tests
1. Unit and integration tests:
   ```bash
   go test -v ./internal/engine -run "TestSEC01|TestUserPerm"
   go test -v ./internal/api -run "TestSEC01"
   ```
2. Full test suite:
   ```bash
   make test
   ```
3. Translation and code checks:
   ```bash
   make check
   ```

### Manual Verification
1. Log in to the Desk as a user with Company Alfa permission.
2. Verify that lists, link lookups, and forms only show and accept Company Alfa records.
3. Verify that trying to access a direct URL for a Company Beta record shows a Permission Denied error.
