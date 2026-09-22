# Frappe REST gap analysis and consumer inventory (OPS-11)

Updated: September 17, 2026.

This is the first step of [OPS-11](../ROADMAP.md) (targeted consumer compatibility).
It lists every difference an existing Frappe API consumer would hit when pointed at
ddcore, and gives a template for recording real consumers. **It adds no compatibility
layer.** Adapters and consumer contract tests come later, and only for a consumer that
has been inventoried below.

## Reference versions

| Side | Pinned version | Sources read |
| --- | --- | --- |
| Frappe | `v15.121.0` (latest v15 tag at the time of writing) | [`frappe/api/__init__.py`](https://github.com/frappe/frappe/blob/v15.121.0/frappe/api/__init__.py), [`frappe/api/v1.py`](https://github.com/frappe/frappe/blob/v15.121.0/frappe/api/v1.py), [`frappe/api/v2.py`](https://github.com/frappe/frappe/blob/v15.121.0/frappe/api/v2.py), [`frappe/handler.py`](https://github.com/frappe/frappe/blob/v15.121.0/frappe/handler.py), [`frappe/client.py`](https://github.com/frappe/frappe/blob/v15.121.0/frappe/client.py), [`frappe/auth.py`](https://github.com/frappe/frappe/blob/v15.121.0/frappe/auth.py), [`frappe/app.py`](https://github.com/frappe/frappe/blob/v15.121.0/frappe/app.py), [`frappe/utils/response.py`](https://github.com/frappe/frappe/blob/v15.121.0/frappe/utils/response.py) |
| ddcore | commit `ba08a77` | Paths cited inline; line numbers refer to that commit |

Re-check both sides before building an adapter. Line numbers go stale quickly.

## Gap classes

- **identical**: a Frappe consumer works unchanged.
- **adaptable**: same concept with a different spelling (path, parameter, envelope key).
  A thin HTTP translation can close it without new engine behavior.
- **incompatible**: the behavior differs in a way a translation cannot hide, or can hide
  only by weakening a ddcore guarantee.
- **absent**: ddcore has no equivalent.

Effort uses the roadmap scale: **S** focused, **M** crosses a few layers, **L** cross-cutting.

## 1. Routes and verbs

| Frappe v15 contract | ddcore behavior | Gap | Effort | Notes |
| --- | --- | --- | --- | --- |
| `GET /api/resource/{doctype}` (list), also under `/api/v1/` | `GET /api/resource/{doctype}` (`internal/api/api.go:113`, handler `:668`) | adaptable | S | Same path. Parameters differ (§2, §4). No `/api/v1` alias. |
| `POST /api/resource/{doctype}` (insert) | Same path (`api.go:114`, `:767`) | identical | — | Body shape differs only for form-encoded `data=` (§2). |
| `GET/PUT/DELETE /api/resource/{doctype}/{name}` | Same paths (`api.go:115-117`) | adaptable | S | Frappe's `<path:name>` accepts `/` in names; chi's `{name}` does not. Frappe accepts a trailing slash (`strict_slashes=False`); ddcore does not. |
| `DELETE` answers **202** `{"data": "ok"}` (`v1.py delete_doc`) | **200** `{"data": {"ok": true}}` (`api.go:814`) | adaptable | S | A client that checks `data == "ok"` or status 202 breaks. |
| `PUT` on a child table row saves the parent (`v1.py update_doc`) | Refused: child DocTypes cannot be mutated directly (`api.go:756` `childGuard`) | incompatible | — | Intentional (B02). Consumers must edit the parent. |
| `POST /api/resource/{doctype}/{name}` with `run_method` = whitelisted controller method; `GET` also allowed with `run_method` | `POST /api/resource/{doctype}/{name}/{method}`, where `{method}` is `submit`/`cancel`/`save`/`amend`/`rename`/`run_method`/a declared controller method (`api.go:118`, `:819-888`) | adaptable | S | Different path. No GET form. Result shape differs (§3). |
| `GET/POST /api/method/{dotted.path}` (`handler.execute_cmd`) | `GET/POST /api/method/{path}` (`api.go:123-124`, `:900`) | adaptable | M | Same route, but method names are `app.dir.file.fn` in the app's TypeScript layout. A consumer's Python paths (`erpnext.stock.utils.get_stock_balance`) need a name map. |
| `frappe.client.get_list`, `get_count`, `get`, `get_value`, `get_single_value`, `set_value`, `insert`, `insert_many`, `save`, `rename_doc`, `submit`, `cancel`, `delete`, `bulk_update`, `has_permission`, `get_doc_permissions`, `get_password`, `attach_file`, `validate_link` via `/api/method/frappe.client.*` (`client.py`) | No `frappe.client.*` methods. Nearest equivalents: `/api/resource/*`, `/api/count/{doctype}` (`api.go:112`), the doc method route, `/api/search/link` | absent | M | frappe-client/frappe-js-sdk call `frappe.client.*` heavily. This is the biggest adapter. `get_password` should stay absent: ddcore never returns Password/Vault values over HTTP. |
| `/api/method/login` (`usr`, `pwd`) and `/api/method/logout` | `POST /api/login`, `POST /api/logout` (`api.go:64-65`, `:421`, `:484`) | adaptable | S | Different path. Login response differs (§5). |
| `/api/method/upload_file` (multipart: `file`, `doctype`, `docname`, `fieldname`, `is_private`, `folder`, `file_url`) | `POST /api/upload` (`api.go:74`, `:1262`) | adaptable | S | Different path. `is_private` defaults to **private** in ddcore (`api.go:1280`) and to public in Frappe. No `file_url`/`folder`. Guests can never upload. |
| `/api/method/ping` | `/api/health`, `/healthz` | adaptable | S | Response differs (`{"message": "pong"}`). |
| `/api/v2/document/*`, `/api/v2/method/{doctype}/{method}`, `/api/v2/doctype/{doctype}/meta` and `/count`, `/api/v2/method/run_doc_method` (`v2.py url_rules`) | None. Meta is `/api/meta/{doctype}` (`api.go:72`) with ddcore's own meta shape | absent | M | v2 is newer and less common among existing consumers. Build it only if one uses it. |
| `run_doc_method` on an in-memory document | None | absent | M | ddcore controller methods run on the stored document only. |
| `/files/*`, `/private/files/*` | Same paths (`serveUpload`, `api.go:1352-1400`) | adaptable | — | Paths match, but stored names are random tokens (`randomFileName`). Old `file_url`s stay valid only if an import (DAT-01/PRD-05) keeps them. |
| `/api/method/frappe.desk.reportview.*`, `frappe.desk.query_report.run` | `/api/report/{name}`, `/api/export/{doctype}` | incompatible | M | Report definitions are ddcore-native. Only port what a consumer actually calls. |

## 2. Request payloads

| Frappe v15 contract | ddcore behavior | Gap | Effort | Notes |
| --- | --- | --- | --- | --- |
| `fields` JSON list, supports `count(name) as n`, `` `tabChild`.`field` ``, dict entries for child fields, `*` | JSON list of plain field names (`api.go:680-686`). Defaults to `["name"]`, as in Frappe (`engine/query.go:281`) | adaptable | M | Aggregates and SQL expressions in `fields` are not parsed. Restricted fields are refused (SEC-02). |
| `filters`: list of `[field, op, value]`, `[doctype, field, op, value]`, dict `{field: value}` or `{field: [op, value]}` | All four shapes (`internal/db/query.go:139` `ParseFilters`). A 4-element filter becomes `Child.field`, which must be a child table of the listed DocType (`engine/query.go` `filterSQL`) | adaptable | S | A 4-element filter that names the **same** DocType (`[["Task","name","like","%005"]]`, the example in `frappe/api/__init__.py`) becomes `Task.name` and is rejected as "not a child table". Fix in the adapter or in `ParseFilters`. |
| Operators: `=`, `!=`, `>`, `<`, `>=`, `<=`, `like`, `not like`, `in`, `not in`, `between`, `is` (`set`/`not set`), `descendants of`, `not descendants of`, `ancestors of`, `not ancestors of`, `timespan`, `fiscal year`, `not between` | `=`, `!=`, `>`, `>=`, `<`, `<=`, `like`, `not like`, `in`, `not in`, `between`, `is`, `set`, `not set`, and the tree operators `descendants of`, `descendants of (inclusive)`, `not descendants of`, `ancestors of`, `not ancestors of` (`db/query.go`) | adaptable | S | The tree operators walk the hierarchy since DAT-07 (trees); before that `descendants of` compiled as `=`, which was silently wrong. Missing: `not between`, `timespan`, `fiscal year`. `like` is case- and accent-insensitive (`ILIKE`), unlike MariaDB collation in some setups. |
| `or_filters` | Supported (`api.go:676`) | identical | — | |
| `order_by` accepts SQL like `` `tabTask`.`modified` desc `` | `field [asc|desc]` list, validated (`db/query.go` `ParseOrderBy`) | adaptable | S | Backticks around a bare field are stripped. Table-qualified names are rejected. |
| `group_by` | Supported (`api.go:695`) | identical | — | Subject to field permissions. |
| `as_dict=0` returns rows as arrays | Always objects | absent | S | |
| `expand` / `expand_links=1` inline linked documents | None. `with_titles=1` returns link titles only (`api.go:714`) | absent | M | |
| `debug=1` | None | absent | — | Do not add: it exposes SQL. |
| Body: JSON, form-encoded, or `data=<json>` form field (`v1.py get_request_form_data`) | JSON only (`api.go` `readJSON`). `/api/login` also accepts form fields (`api.go:433`) | adaptable | S | Python `requests` consumers often send `data=json.dumps(...)`. |
| GET method args: every value is a string; repeated keys are lists | First value of each key, as a string (`api.go:915`) | adaptable | S | Same outcome for most consumers. Typed arguments (numbers, JSON) must be parsed by the app method. |
| Insert with an explicit `name` sets `flags.name_set` (v2) | Naming follows the DocType's `autoname`. Whether a client-supplied `name` is honored depends on it | incompatible | — | Check per DocType for each consumer that sets names. |
| `modified` in an update body is not required | If present, it must match the stored value, otherwise 409 (`engine/doc.go:607-609`) | identical | — | Same optimistic check as Frappe's `check_if_latest`. |

## 3. Response envelopes and serialization

| Frappe v15 contract | ddcore behavior | Gap | Effort | Notes |
| --- | --- | --- | --- | --- |
| `/api/resource/*` answers `{"data": ...}` | `{"data": ..., "messages": [...]}` (`api.go:220-223`) | identical | — | `messages` is extra and ignorable. |
| `/api/method/*` answers `{"message": ...}`, and `_server_messages` for `msgprint` | `{"data": ...}` (`api.go:900-940` through `run`) | adaptable | S | **Most common break.** frappe-client reads `.message` from every method call. |
| Doc method via v1 returns the method's return value as `data`. v2 adds the document to `docs` | `{"data": {"result": ..., "doc": {...}}}` (`api.go:883-886`). Submit/cancel/save return the document | adaptable | S | |
| Documents carry `name`, `owner`, `creation`, `modified`, `modified_by`, `docstatus`, `idx`, `doctype`; child rows add `parent`, `parentfield`, `parenttype` | Same standard columns (`engine/doc.go:456`, `db/schema.go` `stdColumns`) | identical | — | ddcore adds workflow and link-title enrichment keys on `get` (`api.go:743-747`). Clients that round-trip whole documents should drop unknown keys. |
| Datetime `"2026-09-17 10:00:00.123456"` (site timezone, no offset); date `"YYYY-MM-DD"`; time `"HH:MM:SS"` | Datetime RFC 3339 with offset (`internal/db/types.go:48-49`); date and time the same as Frappe (`types.go:21-29`) | adaptable | S | Parsers that `strptime` the Frappe format break. |
| Currency/Float/Int as JSON numbers | Numerics as float64 (`types.go` `Normalize`) | identical | — | Large decimals lose precision the same way. |
| Check fields as `0`/`1` | Booleans: the column is `boolean` (`internal/meta/meta.go:28-29`) | adaptable | S | Clients comparing `== 1` break. |
| Password fields: `*****` placeholder | Removed from the response (`engine.RedactPassword`, `api.go:702`) | incompatible | — | Intentional (SEC-06). |

## 4. Pagination

| Frappe v15 contract | ddcore behavior | Gap | Effort | Notes |
| --- | --- | --- | --- | --- |
| `limit_start` / `limit_page_length` (v1); `limit` also accepted as the page length; v2 `start` / `limit` | `start` / `limit` only (`api.go:690-694`) | adaptable | S | A v1 client sending `limit_start` always gets page one: **silent duplication or a stalled pager**. High priority. |
| Default page length 20; `limit_page_length=0` means unlimited | Default 20. `limit=0` also means 20 (`api.go:691`) | incompatible | S | "Fetch everything" loops that pass 0 receive 20 rows with no error. Decide whether an adapter maps 0 to a capped maximum or refuses it. |
| Total count via `frappe.client.get_count` or v2 `/doctype/{dt}/count` | `GET /api/count/{doctype}` or `with_count=1` returning `{rows, count, titles}` (`api.go:707-712`) | adaptable | S | `with_count` changes `data` from a list into an object. |

## 5. Authentication

| Frappe v15 contract | ddcore behavior | Gap | Effort | Notes |
| --- | --- | --- | --- | --- |
| `Authorization: token api_key:api_secret` (`auth.py` `validate_api_key_secret`) | Same header (`api.go:338`, `engine/auth.go:138`) | identical | — | Keys must be reissued: ddcore stores only a secret hash, and Frappe secrets are encrypted per site. |
| `Authorization: Basic base64(key:secret)` | Not accepted. Anything that is not `token …` falls through to cookie auth or Guest (`api.go:338-352`) | adaptable | S | A Basic-auth consumer silently becomes **Guest**, not 401. Guest-visible data would still answer. |
| `Authorization: Bearer <oauth token>` (OAuth2 provider) | Not supported | absent | L | Tied to SEC-05 (SSO/OIDC). |
| Session cookie `sid` from `/api/method/login`; response `{"message": "Logged In", "home_page", "full_name"}` plus `system_user`, `user_id` cookies | Cookie `sid` from `/api/login`; response `{"data": {"ok": true}}` (`api.go:446`) | adaptable | S | Cookie name matches. Login throttling and password policy apply (SEC-04). |
| CSRF: `X-Frappe-CSRF-Token` must match the session's token (browser sessions only) | Any non-empty `X-DDCore-CSRF` or `X-Requested-With` header on non-GET cookie requests (`api.go:344-349`); login and `/api/auth/*` exempt (`auth.go:161`) | adaptable | S | A cookie-based consumer sending `X-Frappe-CSRF-Token` is refused (403). Token-auth consumers are unaffected. |
| `@frappe.whitelist(allow_guest=True)`; `methods=[...]` restricts verbs (`handler.is_valid_http_method`) | `allowGuest` option (`api.go:907`); `roles` option; GET and POST both reach every method | incompatible | S | ddcore does not restrict verbs per method. A GET-callable mutating method is a CSRF risk only for cookie auth, which the header check covers. |
| IP allowlist per user (`restrict_ip`) | None | absent | M | |

## 6. Errors

| Frappe v15 contract | ddcore behavior | Gap | Effort | Notes |
| --- | --- | --- | --- | --- |
| Body `{"exc_type": "ValidationError", "exception": "...", "_server_messages": "[\"{\\\"message\\\": ...}\"]", "exc": "[traceback]"}` (`utils/response.py report_error`) | `{"error": {"type", "message", "title", "key", "args", "extra", "requestId"}}` (`api.go:217`, `internal/cerr/cerr.go:18-31`) | adaptable | S | `error.type` carries the same class names. `_server_messages` is a double-encoded list Frappe clients parse for user text: map it from `error.message`. No traceback is ever sent (intentional). |
| `ValidationError` / `MandatoryError` / `LinkExistsError` → 417 | Same statuses (`cerr.go:65-80`) | identical | — | |
| `PermissionError` → 403; `DoesNotExistError` → 404; `AuthenticationError` → 401 | Same | identical | — | |
| `TimestampMismatchError` → 417 in Frappe; `DuplicateEntryError` → 409 | Both → 409 (`cerr.go:69-70`) | adaptable | S | Retry logic keyed on 417 for timestamp conflicts must look at `type`. |
| Unknown route → `DoesNotExistError` 404 JSON | `DoesNotExistError` 404 JSON for any unknown `/api/*` path (`api.go:1492-1494`); a known path with an unsupported verb (e.g. `PATCH /api/resource/…`) gets chi's plain-text 405 | adaptable | S | Frappe v2 accepts `PATCH` for updates. |
| 429 via rate limiter | `TooManyRequestsError` 429 with `Retry-After` (`api.go:198-200`) | identical | — | |
| Unhandled error → 500 with traceback in `exc` for developers | `InternalError` 500, `requestId` and an `Error Log` row (`api.go:214-216`) | adaptable | — | Consumers that log `exc` get nothing. Use `requestId`. |

## 7. Runtime dependencies

| Frappe dependency | ddcore replacement | Gap | Effort | Notes |
| --- | --- | --- | --- | --- |
| Python whitelisted methods, controllers, hooks (`hooks.py` `doc_events`, `scheduler_events`) | Synchronous TypeScript controllers, services, `defineApp` hooks, scheduler (see [controller API](agent/controller-api.md)) | incompatible | L per app | Port by hand. No Python runtime is allowed in production ([inventory §8](frappe-port-inventory.md)). |
| `frappe.db.get_value/get_all/sql`, Query Builder (pypika) | `ddcore.db.getValue/getList/getAll/sql` on PostgreSQL | adaptable | M | MariaDB-specific SQL (backticks, `IFNULL`, `DATE_FORMAT`, `GROUP_CONCAT`) must be rewritten. `ddcore.db.sql` ignores scopes and shares. |
| Jinja print formats, email templates, `frappe.render_template` | `definePrintTemplate` blocks ([print](agent/print.md)), `mail/*.mail.ts` ([mail](agent/mail.md)) | incompatible | M per template | No Jinja engine. |
| Server Scripts / Client Scripts stored as data | File-based app code and desk scripts (`@ddcore/desk-sdk`) | incompatible | M | Structural code stays in files (product decision). |
| `frappe.enqueue`, RQ workers | `ddcore.enqueue`, Postgres job queue (PRD-04) | adaptable | S | Job arguments must be JSON. |
| Python packages imported by apps (pandas, requests, openpyxl, …) | No Python or npm packages at runtime. HTTP through `ddcore.http.*` ([controller API](agent/controller-api.md)); anything else needs pure-TypeScript code bundled with the app, a Go host function, or removal | incompatible | varies | List each package in the consumer inventory. |
| Node/Python **client** libraries used by consumers: `frappe-client` (Python), `frappe-js-sdk`, `frappe-react-sdk`, `frappe-ui` `createResource` | None. They target the Frappe contract above | adaptable | see §8 | These are consumers, not server dependencies. The adapter set below targets them. |
| socket.io realtime (`frappe.publish_realtime`, `doc_update`, `list_update`) | SSE `GET /api/events` (`api.go:1192`) with `ddcore.publish` | incompatible | M | Different transport and event names. frappe-react-sdk `useFrappeDocumentEventListener` will not connect. |

## 8. Summary

The gaps a typical Frappe REST consumer hits first, most likely first:

1. **`message` vs `data`** on `/api/method/*` (§3). Every frappe-client call breaks.
2. **`frappe.client.*` absent** (§1). Covers most generic CRUD from SDKs.
3. **`limit_start`/`limit_page_length` ignored** (§4). Fails silently: pagers loop or stall.
   `limit_page_length=0` returns 20 rows without error.
4. **Error body shape** (§6). `_server_messages`/`exc_type` are missing, so clients show a generic error.
5. **Login path and response** (§1, §5). `/api/method/login` returns 404.
6. **Basic auth downgrades to Guest** (§5). Silent.
7. **Datetime format and Check booleans** (§3). Parsing and comparison bugs.
8. **Method name mapping** for app-specific Python methods (§1). Needed per consumer, can't be generic.
9. Upload path and default privacy, form-encoded `data=` bodies, DELETE status (§1, §2).

**Smallest adapter set for a frappe-client/frappe-js-sdk consumer.** An opt-in route group
(for example `/api/v1/*`, or the Frappe paths themselves behind a site flag) that:

- renames list parameters (`limit_start`→`start`, `limit_page_length`/`limit`→`limit`, with
  an explicit cap for 0) and accepts the same-DocType 4-element filter;
- wraps `/api/method/*` results as `{"message": ...}`, and translates `error` into
  `exc_type` plus `_server_messages`;
- implements `frappe.client.get_list`, `get_count`, `get`, `get_value`, `set_value`,
  `insert`, `save`, `submit`, `cancel`, `delete`, `rename_doc` over the existing engine calls
  (no new permission paths);
- aliases `/api/method/login`, `logout`, `upload_file`, `ping`;
- accepts `Authorization: Basic`, form-encoded bodies and `data=`.

Estimated at **M**. Everything else (v2 routes, realtime, `expand`, reports, OAuth, trees)
stays out until a named consumer needs it. Do not port `get_password`, `debug`, tracebacks
in `exc`, or verb-less CSRF bypasses: each would weaken a ddcore guarantee.

## 9. Consumer inventory

**No consumer has been audited yet.** No adapter work starts until at least one row
below is complete and has an owner.

| Consumer | Owner | Source and pinned version | Calls used (route · verb · params) | Auth mode | Pagination | Error handling relied on | Runtime/library deps | Critical flows | Gaps hit (§ refs) | Status |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| _none yet_ | | | | | | | | | | |

### How to fill a row

1. Pin the consumer: repository URL plus commit or release tag, or app version for
   closed clients. Record the Frappe version it was built against.
2. Collect its calls from the source:
   ```bash
   grep -rnE "/api/(resource|method|v2)|frappe\.call|frappe\.client|frappe_client|FrappeApp|useFrappe|createResource|upload_file" <consumer-src>
   ```
   If the source is not available, record traffic from the current Frappe site's access
   log or a proxy for a representative period.
3. For each call, note the parameters, the response fields it reads, and how it handles
   errors and empty pages. Link each one to a row in §1–§7.
4. List server-side dependencies the consumer relies on indirectly: custom whitelisted
   methods, Server Scripts, hooks, Jinja templates, Python packages.
5. Name the critical flows the consumer must keep (for example "create Sales Order from
   the mobile app", "nightly stock sync").

### Acceptance for the next step

An adapter for a consumer is done when:

- a contract test suite replays that consumer's recorded requests, at its pinned version,
  against ddcore and checks the response fields and statuses the consumer reads;
- each critical flow from its row passes end to end against a rehearsal instance with
  outgoing effects disabled;
- only the routes, payloads, auth, errors and pagination rules listed in its row are
  adapted, and each adaptation uses existing engine permission paths.
