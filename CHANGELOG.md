# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.27](docs/changelog/0.27.md), [0.26](docs/changelog/0.26.md), [0.25](docs/changelog/0.25.md), [0.24](docs/changelog/0.24.md), [0.23](docs/changelog/0.23.md), [0.22](docs/changelog/0.22.md), [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

### Fixed

- **A reload that is refused no longer leaks the runtime pool.** When `Engine.Load` refused an app set (an invalid DocType, a failed extension, an i18n error), some of those paths left the freshly built JS runtime pool open; the pool is now closed on every refusal, and so is one whose first runtime fails to start or to report its metadata.
- **A workflow transition's response now carries the computed fields.** `POST /api/workflow/apply` returned the document without running `onLoad`, so a computed field was missing from it while `GET` showed it.
- **Insert and tree-children routes decode the DocType name in the URL.** They read the raw route parameter, so a percent-encoded name did not match its DocType and the insert response skipped redaction.
- **A Select whose options are a newline-separated string is now extracted for translation.** `ddcore i18n extract` skipped that form (which `Validate` accepts), so its options never reached the catalogue and rendered in English on a translated screen.

## 0.28.3 — 2026-10-17

### Fixed

- **A number card whose value is a sentence no longer grows into a tall card.** A `formatted`
  value longer than 16 characters, such as `226 granted · 885 denied · 3170 pending`, is now shown
  at 15px instead of 22px. Plain numbers and amounts keep the large size.

## 0.28.2 — 2026-10-17

### Fixed

- **Chart X-axis labels no longer overlap.** In a workspace or report chart with many points,
  such as 30 daily `dd/mm` labels, every label used to be drawn and they ran into each other. The
  chart now shows one label every few points, depending on how much room the longest label needs,
  and always keeps the last point's label. Every bar or point still shows its full label in the
  tooltip.

## 0.28.1 — 2026-10-16

Opens the 0.28 series. Nothing breaks: a site pinned to `<0.28.0` only widens its `ddcore` range
in ddcore.json (and its Docker image tag) to take it.

### Added

- **`ddcore.ai`: the site's language model from server code.** The provider is an
  OpenAI-compatible Responses API. The key comes from `DDCORE_AI_API_KEY`. The models and the
  address come from `ai` in ddcore.json (`model`, `transcriptionModel`, `baseUrl`,
  `reasoningEffort`, `timeoutSeconds`), or from `DDCORE_AI_*`; the defaults are `gpt-6-luna` and
  `gpt-transcribe`.
  - `ddcore.ai.respond({ instructions, input, tools?, schema?, … })` returns `text`, `calls`,
    `json` and `usage`, plus `output` for a tool loop to replay untouched, encrypted reasoning
    included.
  - `ddcore.ai.transcribe(base64, { filename, language? })` turns an audio into text.
  - `ddcore.ai.image(base64, type)` makes an image part for the input.
  - Calls retry a 429, a 5xx or a dropped connection within their timeout. Errors are typed, and
    the prompt is never logged.
  - Inside `ddcore test`, every call goes to the new `ddcore.test.fakeAi({ respond, transcribe })`,
    which receives the exact body a real call would send. A test without it throws instead of
    spending tokens.

  See `docs/agent/ai.md`.
- **Feedback: Fill in with AI, and several audio notes.**
  - The dialog now shows the attachments right after the type. **Record audio** sits inside that
    card, and records as many notes as fit in the 10 files (`voice-note-1.webm`, `-2`…).
  - On a site whose AI is on, **Fill in with AI** transcribes the notes and gives the model the
    transcripts, the screenshots, what was typed, the page, the context the user agreed to send,
    and the site's apps and current DocType. It fills in the title, the description and the
    type's fields, which the user reviews (and can undo) before sending.
  - The new `POST /api/feedback/assist` writes nothing and allows 10 requests per user per hour.
    The boot flag is `site.feedbackAssist`, and `feedback.assist` (`DDCORE_FEEDBACK_ASSIST`)
    turns it off.
  - The transcripts are saved on the new read-only field `Feedback.transcript`, and they go in
    the mail too.

  See `docs/agent/feedback.md`.

<!-- #endregion releases -->
