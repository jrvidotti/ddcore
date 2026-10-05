# Feedback

Every desk user can tell the developers of the site's apps about a bug, an improvement or a
feature they miss, from **Feedback** in the user menu (the avatar at the foot of the sidebar,
or the avatar in the mobile header). The feature is part of the framework: an app gets it
without writing anything, and a site can turn it off.

## What the user sees

A dialog with two tabs.

**Send** asks for the type first, then the fields of that type:

| Type | Fields |
| --- | --- |
| all | `title` (required), `description` (required) |
| `Bug` | `severity` (`Low`, `Medium`, `High`, `Critical`), `steps_to_reproduce`, `expected_result`, `actual_result` |
| `Improvement` | `current_behavior`, `suggested_improvement` |
| `Feature Request` | `problem`, `expected_benefit` |

Below them:

- **Attachments.** Up to 10 files, picked, dropped on the dialog, or pasted from the clipboard (a
  screenshot is pasted with Ctrl+V). Together with the rest of the request they stay under the
  upload limit (50 MB by default).
- **An audio note**, recorded in the browser where it can (a secure context with a microphone)
  and listened to before it is sent. It is one more file, `voice-note.webm` (`.m4a` on Safari).
- **Send the address of this page**, checked: the URL the user is on.
- **Send context data**, checked: user, roles, language, site name and version, tenant, apps,
  the route's workspace, DocType and id, viewport, user agent, timezone, and the last client
  errors (failed requests with their request id, uncaught errors). "See what will be sent"
  shows the JSON before it goes.

**My feedback** lists what the user sent, newest first, with the status and the developer's
response.

An app's desk script can open the dialog pre-filled, for a "Report a problem" button of its own:

```ts
ddcore.ui.openFeedback({ type: "Bug", title: "Invoice total is wrong" });
```

## Where it lands

Each submission is a document of the core DocType **`Feedback`**, its files `File` documents
attached to it (private). Only `System Manager` reads `Feedback`: it shows under **System** in
the sidebar, with `status` (`New`, `In Review`, `Planned`, `Done`, `Won't Do`) and `response`
for the developer to fill in. The author never reads the DocType: they follow their own
through `GET /api/feedback/mine`, which returns the status and the response.

The document also records who sent it and from where: `reported_by` (the user id),
`reporter_name`, `reporter_email`, `source_tenant`, `app_version`, `page_url` and `context`.

**On a site with tenancy, every feedback is written in the platform space**, whichever space
its author works in, with `source_tenant` naming the tenant. The developers of the apps are the
platform's; a tenant's own System Manager does not see their users' feedback. The write runs as
`Admin` because a tenant's user cannot write in the platform space.

## Who is told

When one arrives:

- **Every System Manager of the platform** gets it in the desk inbox (the bell), and the email
  copy every inbox notification has. The author is left out when they are one.
- **The site's feedback addresses** get a mail with everything typed and the files attached,
  as far as `DDCORE_MAIL_MAX_ATTACHMENT` allows; the mail says how many did not fit, and its
  button opens the document. A mail that cannot be queued is logged and never loses the
  feedback.
- **Webhooks**: a `Webhook` on `Feedback` with `on_insert`, in the platform space, fires like on
  any other DocType — the way to open an issue in a tracker (see `webhooks`).

## Configuration

In `ddcore.json`:

```json
{ "feedback": { "enabled": true, "to": ["dev@example.com"] } }
```

| Key | Environment | Default | Meaning |
| --- | --- | --- | --- |
| `feedback.enabled` | `DDCORE_FEEDBACK` (`0`/`1`) | on | Off hides the menu item, and the endpoints answer 404 |
| `feedback.to` | `DDCORE_FEEDBACK_TO` (comma-separated) | none | Addresses mailed a copy of each feedback |

The boot carries `site.feedback`, true for a signed-in desk user when the feature is on.

## Limits

- 10 files per feedback, all of them within the upload limit.
- 20 feedbacks per user per hour.
- `title` up to 200 characters, each text up to 20,000, `page_url` up to 2,048, `context` up to
  64 KiB and a JSON object.
- Only the fields of the chosen type are taken; anything else (a `status`, an `owner`) is
  refused.

## HTTP

Both need a signed-in desk user; a Website User is refused.

- `POST /api/feedback`, `multipart/form-data`: `data`, the fields as a JSON object
  (`feedback_type`, `title`, `description`, the type's fields, optional `page_url` and
  `context`), and up to 10 parts named `files`. Answers `{ "data": { "id": "…" } }`.
- `GET /api/feedback/mine`: `{ "data": [{ "id", "title", "feedback_type", "status",
  "response", "creation" }] }`, the last 50.

Audio files (`.webm`, `.ogg`, `.oga`, `.opus`, `.m4a`, `.mp3`, `.wav`) are served in place with
their audio type, like images and PDFs, so the document's sidebar plays them; everything else
is still a download.
