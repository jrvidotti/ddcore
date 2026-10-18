# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.28](docs/changelog/0.28.md), [0.27](docs/changelog/0.27.md), [0.26](docs/changelog/0.26.md), [0.25](docs/changelog/0.25.md), [0.24](docs/changelog/0.24.md), [0.23](docs/changelog/0.23.md), [0.22](docs/changelog/0.22.md), [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

## 0.29.1 — 2026-10-18

No change an app would notice. Three API tests that still expected workspace entries a reader
cannot open now follow the reader filter (#131), so the full suite passes again.

## 0.29.0 — 2026-10-18

### Breaking

- **A `Column Break` field is no longer dropped with a warning.** The fieldtype was removed in 0.4.0 and the loader quietly discarded such fields from older apps; it now refuses it like any unknown fieldtype, and a refused DocType stops the whole site from loading: `ddcore dev`, `ddcore start` and `ddcore migrate` fail with `<DocType>: invalid fieldtype "Column Break" on field …`. **Upgrade:** delete the `Column Break` fields from your DocTypes; a field's width comes from its `width`.

### Fixed

- **A reload that is refused no longer leaks the runtime pool.** When `Engine.Load` refused an app set (an invalid DocType, a failed extension, an i18n error), some of those paths left the freshly built JS runtime pool open; the pool is now closed on every refusal, and so is one whose first runtime fails to start or to report its metadata.
- **A workflow transition's response now carries the computed fields.** `POST /api/workflow/apply` returned the document without running `onLoad`, so a computed field was missing from it while `GET` showed it.
- **Insert and tree-children routes decode the DocType name in the URL.** They read the raw route parameter, so a percent-encoded name did not match its DocType and the insert response skipped redaction.
- **A Select whose options are a newline-separated string is now extracted for translation.** `ddcore i18n extract` skipped that form (which `Validate` accepts), so its options never reached the catalogue and rendered in English on a translated screen. It now also extracts list items that are not strings (numbers, say), so an app's `ddcore i18n extract --check` may report new missing keys: run `ddcore i18n extract` after upgrading and translate them.
- **`ddcore restore` writes the maintenance flag the way the engine does.** It carried its own copy of the SQL, which reset `since` when the flag was set again while already on and created the ops tables on the confined pool of a site with tenancy (#112). It now shares the engine's code.
- **`ddcore user unlock` now records an `account.unlock` audit event and trims what was typed.** The command line cleared the counters without leaving the trace the desk's unlock leaves, and it did not trim the address the way sign-in does, so a stray space cleared nothing. Only the command changed; unlocking from the desk still goes through `core/services/users.ts`.
- **`ddcore import` stops cleanly on SIGTERM.** It listened for Ctrl-C only, so a `kill` or a container stop left a half-finished batch instead of cancelling the run; it now shares the signal handling `ddcore restore` uses.
- **`ddcore jobs --since` accepts days.** `audit` took `7d` and `jobs` refused it; both now parse `--since` the same way (a Go duration or a number of days).
- **`ddcore webhooks list` handles `-h` and flags after arguments like the other commands.** It used a bare `flag.FlagSet`; it now goes through the shared flag helpers, and its timestamp and error columns are formatted as `ddcore jobs list` does.
- **Three CLI and loader messages that were in Portuguese are in English.** `ddcore user add` usage, the location line of a validation finding (`in <file>`) and the "DocType defined twice" error.
- **A record's history shows its status in the user's language.** The `docstatus` row of a Version was hard-coded in Portuguese ("Rascunho", "Enviado", "Cancelado") whatever the language; it now goes through the catalogue (`Draft`, `Submitted`, `Cancelled`).
- **Five desk labels that were hard-coded in Portuguese now follow the language.** A Link field's placeholder, clear and open-record labels ("Escolha o tipo antes", "Limpar …", "Abrir …"), and the Close and Clear buttons' accessible names in dialogs, the form's side panel and the history search.
- **The print view's language picker lists the site's languages.** It offered a fixed Portuguese, English and Spanish list whatever the site had installed; it now uses the languages the server reports at boot, as the profile does.
- **A new document's defaults use the site's day, and Datetime defaults are an instant.** A `default: "Today"` was resolved in the browser's timezone, so a form opened near midnight could start a day off the server's `today()`; the same went for the due date marked overdue in a document's side panel, which counted in UTC. Both now use the site's timezone. The form also takes the server's `today` and `now` keywords (a Datetime starts at the current instant, no longer a bare date) and `__user`, which it used to show as text.
- **The desk no longer keeps every server message in memory.** `onMessage` also removed the wrong listener when its unsubscribe function ran twice; listeners now live in a set and the unread message list is gone.

<!-- #endregion releases -->
