# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.29](docs/changelog/0.29.md), [0.28](docs/changelog/0.28.md), [0.27](docs/changelog/0.27.md), [0.26](docs/changelog/0.26.md), [0.25](docs/changelog/0.25.md), [0.24](docs/changelog/0.24.md), [0.23](docs/changelog/0.23.md), [0.22](docs/changelog/0.22.md), [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

### Fixed

- **The history filter also searches removed rows.** Searching a document's full history for a value that only a removed child-table row held found nothing; added, modified and removed rows are all matched now.
- **Security: a `report` right no longer widens a list beyond what `read` allows.** Listings counted a permission row with `report` (and no `read`) as a read grant, while opening a document did not. A role with `report` but not `read`, once one document of the DocType was shared with the user, listed every document of it, and every child row of its tables; a `report` row beside an `ifOwner` read listed other users' documents too. Lists (including the tree view and virtual DocTypes) now follow the same rule as reading one document: only `read` rows grant it, `report` alone grants nothing to list. If a role was meant to list a DocType, give it `read`.
- **Security: a child DocType lists only the rows of parent documents the user can list.** A listing of a child DocType (`getList`, `count`, `GET /api/resource/<child>`) checked only that the user could read *some* parent DocType, so a user limited by User Permission scopes listed the rows of every parent document out of scope when the rule was `applicable_for` the parent or the child table had no `Link` to the scoped DocType (line items without a `company`, say); the parent controller's `permissionQuery` was ignored, and so was the permission level of the `Table` field holding the rows. A row is now listed only when its parent document passes everything a list of the parent applies for the user (roles, `ifOwner`, shares, scopes, `permissionQuery`, portal), its `Table` field is readable at its level, and its own `Link` fields pass the rules for the parent's `applicable_for`; reading one row directly checks the field level and those row rules as well. Upgrade note: users limited by scopes, or listing the children of a parent whose controller has a `permissionQuery`, now see only the rows of parents they can list; rows of a `Table` field the user cannot read at its permission level are no longer listed or readable directly; Admin and `ignorePermissions` reads are unchanged.
- **An error raised while checking a permission is reported as that error.** A controller `hasPermission` that throws (or a failed lookup behind the check) made submitting on insert, deleting, renaming, amending and merging answer "No permission"; the error now surfaces as itself, as it already did for reading, saving and creating.

## 0.30.0 — 2026-10-19

### Fixed

- **Assignment timeline comments are translated.** "assigned this document to", "completed assignment", "revoked assignment for" and "reopened assignment" were always written in English; they are now written in the language of the user who makes the change (pt-BR included).
- **Moving a node of a tree DocType reaches the other processes.** The read cache of the subtree was dropped only in the process that saved the move; replicas and workers kept answering from it until it expired.
- **The `scaffold_doctype` MCP description lists the `Report` fieldtype.** It named every other fieldtype but that one; a test now keeps the description and the `Fieldtype` union in the SDK in step with the fieldtypes the framework accepts.
- **`ddcore.redact` refuses an unknown doctype instead of returning the document unredacted.** The same holds for the MCP `list_docs` and `call_method` tools, notifications, webhooks and the portal: a doctype that cannot be looked up is now an error, not a pass-through.
- **The change history shows values the way the lists do.** A Select now reads as its label, a Link as its title, and a Currency at its field's precision instead of the site default; Dates, Months and the other types it shared with the lists follow the same rules.
- **Escape in an open options list inside a dialog closes only the list.** A Link field's list or a Table MultiSelect's list, opened in a dialog such as Assign or New Task, used to take the whole dialog with it on Escape.
- **The due date of an assignment and of a new To-Do task follows the site's date format.** Both forms used the browser's native date input, in the browser's locale; they now use the desk's date control like every other date field. The New Task form also lists the assignee first, like the Assign form.
- **The `generate_types` MCP tool reports a failed write.** It dropped the error and listed only the files it had written; it now fails with the error, and `migrate` adds a `typesError` field when the typings could not be written (the migration itself is unaffected). Writing stops at the first app it cannot write, so the apps after it (in `migrate` as well) are not written either.
- **A mail delivery whose outcome could not be recorded is logged, and its error column is empty (NULL) on success.** The failed `UPDATE` was dropped without a trace, and a successful delivery stored an empty string; mail and webhook errors are now both cut at 1000 characters (a webhook's was cut at 1000 bytes, which could split a character and lose the outcome), a failed status update of a mail is no longer dropped silently, and `sent_at` is stamped by the database.
- **A database failure in the tenancy check surfaces as an error.** `tenant.enter`, `InTenant` and `EnterTenant` treated it as "tenancy is off"; they now return the error.

### Changed

- **The desk's dialogs close the same way.** Share, Assign, Shortcuts, New Task, Data Import, Feedback and the change-history dialog all close on Escape, on a click on the backdrop and from the close button, and Escape in any of them no longer reaches the page behind. The change-history dialog gains the backdrop click it lacked. They share one `Modal` component.
- **One dismiss action for the desk's popovers.** The date, month and colour pickers, the Columns menu, the sidebar menus and the space badge share one "close on a press outside or Escape" action. In the portal, Escape now closes the account menu; in the sidebar an Escape from inside an open menu closes only that menu; and an Escape pressed outside an open colour picker closes it and returns the focus to its swatch. The date, month and colour pickers now also close when a touch starts outside them, even a scroll.
- **`ddcore types` no longer opens the database.** It compiles the apps and writes `.ddcore/types.d.ts` without connecting, so it works with Postgres down or no dsn configured (`ddcore migrate` still ends by writing the types, from the engine it already has open instead of loading the apps a second time). It also ignores `--tenant` and skips the site-version check now.
- **`ddcore i18n -help` prints the usage and exits 0** like the other commands; it used to fail with `unknown i18n subcommand`.

<!-- #endregion releases -->
