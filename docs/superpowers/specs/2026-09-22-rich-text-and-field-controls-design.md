# Design: Rich Text and Additional Field Controls (DAT-08)

This document explains **why** the rich text contract and the new field controls are
shaped the way they are. The API reference is
[`docs/agent/fieldtypes.md`](../../agent/fieldtypes.md).

## Starting point

- **`Text Editor` was a textarea.** It validated nothing, sanitized nothing and stored
  whatever arrived. Print escaped it, so a value holding markup printed as literal tags,
  and the one framework user of the type — `Comment.content` — was shown as escaped text.
- **There was no sanitizer anywhere.** Neither Go nor the desk had one, and
  `internal/engine/print.go:sanitizeDocForPrint` only strips secret and restricted fields.
- **The fieldtype list is repeated.** `ValidFieldTypes` and `ColumnType`
  (`internal/meta/meta.go`), `castValueWith` (`internal/engine/doc.go`), `tsType`
  (`internal/typegen/typegen.go`), the SDK union (`packages/sdk/src/types.ts`), the MCP
  scaffold description and the desk's width table each hold their own copy. A new type
  touches all of them.
- **`castValueWith` is the single write path.** REST, MCP, `dbSet`, defaults and fixtures
  all arrive there, and it also runs on both sides of the read-only, allow-on-submit and
  permlevel comparisons (`sameFieldValue`, `internal/engine/fieldperm.go`).
- **Apps already expected some of this.** `desk/src/lib/components/history.ts` formats an
  `Attach Image` diff for a fieldtype that did not exist.

## Key Decisions

### 1. `Text Editor` stores sanitized HTML, and the server never refuses it

The server cleans the markup with an allowlist and saves the result. It does not reject a
document because its HTML carried something extra. The stored value is what the response
returns, so the client sees exactly what was kept.

**Rationale:** an editor's output, a paste from a word processor and an API or MCP client
all carry markup nobody typed on purpose. Refusing it would fail writes whose *content* is
fine, and the app author cannot fix what a user pasted. Cleaning is not a warning to act
on; it is the contract. Storing the clean value also means the cleaning is paid once per
write instead of on every read, print and export.

### 2. Sanitizing is idempotent

`Sanitize(Sanitize(x)) == Sanitize(x)` is a property test, not a hope. The same holds for
the whole write path: a document opened in the Desk and saved untouched must produce the
same bytes, so it records no Version and trips no read-only comparison.

**Rationale:** the value is re-cast on every save and compared with itself by
`sameFieldValue`. A sanitizer that adds one `rel` token or re-encodes one entity per pass
would make every submitted document unsaveable and fill the timeline with empty diffs.

### 3. The allowlist is what the editor can produce

The Desk edits rich text with Tiptap, and the server's allowlist is exactly the set that
the Tiptap setup round-trips: `p br hr blockquote h1`–`h4 strong b em i u s del code pre ul
ol[start] li`, `a[href,title]`, `img[src,alt,title,width,height]` and `class="language-*"`
on `code`. `style`, event handlers, `script`, `iframe`, `svg`, `form`, `data:` and
`javascript:` URLs are always removed. Tables are not allowed in rich text for now.

**Rationale:** any tag the server keeps but the editor cannot represent is data a user
destroys by opening the form and saving. The narrower set is the honest one. Tables are
the clearest example, which is why they wait for the editor extension rather than being
allowed first.

### 4. An image in rich text is a local file

`img src` must match `/files/…` or `/private/files/…`. External URLs, `https` included,
are refused.

**Rationale:** the PDF renderer fetches a document's images from the server
(`internal/print/pdf.go`), so an external `src` turns any user who can write a rich-text
field into a request generator against arbitrary addresses (SSRF), and turns any reader
into a tracking-pixel hit. Allowing `https` is a later opt-in, not a default.

### 5. Legacy plain text is recognised, not guessed

Values written before this change are plain text. `LooksLikeHTML` answers true only when
the value carries a tag from the allowlist, so `a < b` and `<not-a-tag>` stay text. Plain
text is escaped, blank lines become paragraphs and single newlines become `<br>`. The
conversion happens where the value is read (desk, print) and is written back on the next
save.

**Rationale:** the alternative — treating every stored value as markup — would silently
delete a `<` and everything after it in real data. There is no data migration because the
column does not change and the read path already has to handle both, so a backfill would
only add a failure mode.

### 6. Empty markup is null

After cleaning, markup with no text and no image (`<p></p>`, `<p><br></p>`) is stored as
`null`.

**Rationale:** every editor leaves an empty paragraph behind when the user clears a field.
Without this, `reqd` would accept it and an app's `if (!doc.notes)` would be wrong.

### 7. `Markdown Editor` stores its source

The Markdown source is stored as written and rendered to HTML only for display and print,
through goldmark and then the same sanitizer with a slightly wider policy (headings 5–6
and GFM tables).

**Rationale:** Markdown is the one format where the source *is* the user's document —
round-tripping it through HTML would rewrite their text. Rendering late also means an
improvement to the renderer reaches old content. Sanitizing the rendered HTML is not
optional: goldmark passes raw HTML through unless it is told otherwise, and it is told
otherwise here as well.

### 8. `Duration` and `Rating` are integers in a `bigint`

`Duration` stores whole seconds, `Rating` stores 0..N stars with N from `options` (1–10,
default 5). `options: ["hideDays", "hideSeconds"]` changes a duration's display only.

**Rationale:** exact integers sum correctly in a report and filter naturally (`rating >=
4`), and `Int` converts to either with no DDL. Frappe's 0..1 float rating cannot represent
3 of 5 stars exactly; the price of integers is that changing N does not rescale stored
values, which the reference documents.

### 9. `Code`, `Attach Image` and `Color` validate what their control assumes

`Code` keeps its text byte for byte — leading whitespace is meaning — and its `options`
names a language. `Attach Image` is an `Attach` restricted to an image extension, checked
on save *and* at upload. `Color` is normalised to lowercase `#rrggbb`.

**Rationale:** each of these is a control over a `text` column; without server validation
the control is a suggestion, and a value typed through the API would break the form that
reads it back. Restricting the upload too means the bytes never land for a field that
would refuse the URL.

### 10. The Desk sanitizes again before rendering

Nothing rendered with `{@html}` skips the desk's own sanitizer, even though the server
already cleaned the value on write.

**Rationale:** the server is the guarantee, but the desk must not depend on every row in
the database having been written by this version of the server. `ddcore.db.sql`, a restore
from an older archive and a direct SQL write all reach the same screen.

## Known limitations

- A private image (`/private/files/…`) does not render in a server-side PDF, the same
  limitation the Letter Head logo has.
- Mail blocks still escape everything; there is no rich-text block in a message.
- `Code` has no syntax highlighting, and rich text has no tables.
- Export carries the stored HTML as-is, which is what round-trip fidelity requires; a
  consumer that wants plain text must strip it.
- Global search still does not look into long text.

## Follow-up, outside this item

`FormView.svelte`, `Dialogs.svelte` and `Toasts.svelte` render developer- and app-supplied
strings with `{@html}` and no sanitizer. That predates this work and is listed here so it
is not lost: the content is authored by a System Manager or by app code, which is why it
is hardening rather than a fix that belongs to DAT-08.
