# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.30](docs/changelog/0.30.md), [0.29](docs/changelog/0.29.md), [0.28](docs/changelog/0.28.md), [0.27](docs/changelog/0.27.md), [0.26](docs/changelog/0.26.md), [0.25](docs/changelog/0.25.md), [0.24](docs/changelog/0.24.md), [0.23](docs/changelog/0.23.md), [0.22](docs/changelog/0.22.md), [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

### Added

- **Server code can render a print template to a PDF `File`** (#136). `ddcore.print.pdf({ doctype, id, format?, letterHead?, language?, pageFormat?, landscape?, filename?, ignorePermissions? })` prints the document as `GET /api/print/{doctype}/{id}/pdf` does, and stores the PDF as a private `File` attached to the document, on the current transaction, so `ddcore.sendMail({ attach: [pdf.id] })` can mail it from a job. Printing needs `read` and attaching needs `write` on the document, unless `ignorePermissions`; the File is named `<format>-<id>.pdf` by default. Without a PDF renderer it throws `UnavailableError`. See `print`.

### Fixed

- **A binary newer than the database's last migration says so** (#135). `ddcore migrate --dry-run` lists the framework's own schema changes — such as the `ddcore_vault.key_id` column 0.31.1 adds — under `framework (before the patches)` instead of answering `nothing to do`; `migrate` counts them in its `ok:` line and records them in `ddcore_migration`, and `ddcore doctor` and `ddcore dev` without `--auto-migrate` report them as pending DDL. `ddcore vault rekey` (MCP: `vault_rekey`), the doctor's vault census and the vault reads and writes stop with `The vault has not been migrated to this version: run ddcore migrate` instead of Postgres's `column "key_id" does not exist`, and a re-key that cannot start records no Audit Event.

## 0.31.1 — 2026-10-22

### Added

- **The vault's master key can be rotated** (#134). `DDCORE_SECRET_KEY_PREVIOUS` holds the old key while a new one is deployed in `DDCORE_SECRET_KEY`: a secret that does not open under the current key is read with the previous one, and every write uses the current one. `ddcore vault rekey` (MCP: `vault_rekey`) then re-encrypts every secret still under the previous key, in every tenant, in batches of one transaction each; `--dry-run` counts the secrets under each key, a secret that opens under neither stops it by name (never by value), and a run is one `vault.rekey` Audit Event with counts only. `ddcore doctor` warns while secrets remain under the previous key, says when it can be removed, and names secrets no key opens. Each `ddcore_vault` row now records a fingerprint of the key that encrypted it (`key_id`, added by `migrate`); rows written before are stamped by the first `rekey`. `ddcore.secret("key_previous")` is refused, as `ddcore.secret("key")` is. See `vault` for the procedure and `backup` for archives taken before a rotation.

## 0.31.0 — 2026-10-19

### Fixed

- **The history filter also searches removed rows.** Searching a document's full history for a value that only a removed child-table row held found nothing; added, modified and removed rows are all matched now.
- **Security: a `report` right no longer widens a list beyond what `read` allows.** Listings counted a permission row with `report` (and no `read`) as a read grant, while opening a document did not. A role with `report` but not `read`, once one document of the DocType was shared with the user, listed every document of it, and every child row of its tables; a `report` row beside an `ifOwner` read listed other users' documents too. Lists (including the tree view and virtual DocTypes) now follow the same rule as reading one document: only `read` rows grant it, `report` alone grants nothing to list. If a role was meant to list a DocType, give it `read`.
- **Security: a child DocType lists only the rows of parent documents the user can list.** A listing of a child DocType (`getList`, `count`, `GET /api/resource/<child>`) checked only that the user could read *some* parent DocType, so a user limited by User Permission scopes listed the rows of every parent document out of scope when the rule was `applicable_for` the parent or the child table had no `Link` to the scoped DocType (line items without a `company`, say); the parent controller's `permissionQuery` was ignored, and so was the permission level of the `Table` field holding the rows. A row is now listed only when its parent document passes everything a list of the parent applies for the user (roles, shares, scopes, `permissionQuery`, portal), its `Table` field is readable at its level, and its own `Link` fields pass the rules for the parent's `applicable_for`; a parent DocType the user reads only as owner (`ifOwner`) still lists no child rows at all. Reading one row directly checks the field level and those row rules as well, and also the rules without `applicable_for` against the row's own `Link` fields. Upgrade note: users limited by scopes, or listing the children of a parent whose controller has a `permissionQuery`, now see only the rows of parents they can list; rows of a `Table` field the user cannot read at its permission level are no longer listed or readable directly; Admin and `ignorePermissions` reads are unchanged.
- **An error raised while checking a permission is reported as that error.** A controller `hasPermission` that throws (or a failed lookup behind the check) made submitting on insert, deleting, renaming, amending and merging answer "No permission"; the error now surfaces as itself, as it already did for reading, saving and creating.

<!-- #endregion releases -->
