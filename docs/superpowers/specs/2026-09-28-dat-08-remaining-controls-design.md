# Design: The Remaining Field Controls — Autocomplete, Barcode, Signature, Geolocation (DAT-08)

This document explains **why** the last four DAT-08 fieldtypes are shaped the way they
are. The API reference is [`docs/agent/fieldtypes.md`](../../agent/fieldtypes.md); the
first round of the item, rich text and the simpler controls, is in
[2026-09-22-rich-text-and-field-controls-design.md](2026-09-22-rich-text-and-field-controls-design.md).

## Starting point

- **The backlog asked for proof, not controls.** The ROADMAP row listed Geolocation,
  Signature, Barcode and Autocomplete and asked for "round-trip fidelity, conversion and
  server validation for each". A control over a column the server does not check is a
  suggestion; a value that changes shape on its way back is a Version on every save.
- **`castValueWith` is still the single write path** (`internal/engine/doc.go`), and still
  runs on both sides of the read-only, allow-on-submit and permlevel comparisons
  (`sameFieldValue`). Whatever it returns for a stored value has to equal what it returns
  for the same value read back.
- **The existing `JSON` type does not meet that bar.** It returns a string on write and
  the driver hands back a decoded map on read, so a document with a JSON field records a
  Version on every save. That is reported separately and not fixed here, but it is why
  Geolocation does not simply reuse it.
- **Two of the four are large.** A signature image and a GeoJSON value run to tens of
  KiB, past the ~2.7 KB Postgres accepts in a btree entry, and heavy for anything that
  reads a column for every row of a list.

## Key Decisions

### 1. Autocomplete is free text, and its suggestions are never enforced

An `Autocomplete` is a `text` column whose `options` are suggestions — a list, or one per
line. The server trims the value and stores `""` as null, and nothing else: any text
saves. The options are not catalogue keys, so they are not translated, and translated
metadata now carries `optionLabels` for a `Select` only. `frm.setDfProperty(field,
"options", list)` replaces the suggestions at runtime.

**Rationale:** that is what distinguishes it from a `Select`. An enforced list is a
Select; a list that only helps the typing is Autocomplete, and Frappe's type means the
same. Trimming (like `Email`, unlike `Data`) keeps "Red" and "Red " from being two
values, and the first save after a `Data` → `Autocomplete` change is compared trimmed, so
it records no whitespace-only Version.

### 2. Barcode stores the text; one Go encoder draws it everywhere

A `Barcode` stores the code's text, never an image. `options` is the symbology —
`Code128` (the default), `EAN-13` or `QR` — and an unknown one fails at load. The server
validates each value for it (printable ASCII up to 80 characters; 12 or 13 digits, the
check digit appended or verified; up to 1000 bytes) and completes a 12-digit EAN-13, so
the label printed later scans as what was stored. `internal/barcode` wraps
boombuler/barcode and draws one SVG path with a quiet zone; the desk preview and portals
fetch it from `GET /api/barcode?symbology=&value=`, and print draws it through the same
package.

**Rationale:** the text is what an app searches, links and imports; the image is derived
and can always be redrawn. One encoder, on the server, means the form's preview and the
printed label cannot disagree, and the desk carries no barcode library. The endpoint reads
nothing from the database, so it is open to any signed-in user, Website Users included.
Camera scanning uses the browser's `BarcodeDetector` where it exists, with no polyfill.

### 3. Signature is a PNG data URL in the row, checked on write and again on print

A `Signature` stores `data:image/png;base64,…` in a `text` column — what Frappe stores, so
an imported Frappe value keeps working. `internal/signature` accepts that exact prefix,
base64 that decodes, real PNG bytes, at most 64 KiB and 2000×1000 pixels. Print checks it
again before it reaches an `<img src>`.

**Rationale:** a file per signature would need its own lifecycle, permissions and
cleanup for something a few KiB large that belongs to exactly one document. Keeping it in
the row keeps it Frappe-compatible and transactional with the document. The strict prefix
is the security boundary: the value lands in `src` on the form and in the PDF, so markup,
`javascript:` or an SVG — which carries script — can never save, and a template that
passes `b.signature` anything else prints nothing.

### 4. Large values are refused where they would be indexed or listed

`meta.BulkyFieldtype` (Signature and Geolocation) makes `migrate` refuse `unique`,
`searchIndex`, `inStandardFilter`, and the field as `titleField`, `sortField`,
`searchFields`, `linkSubtitle`, a `uniqueKeys` member or a Table's `gridSort`. A
Signature is also refused as `inListView` outside a child DocType; a Geolocation is
allowed there, because the list shows its summary.

**Rationale:** each of those either builds a btree over the value or reads it for every
row of a list, a search or a Link dropdown. Refusing at load names the field while the
author is looking at it; the alternative is a Postgres error at `migrate` or a list that
pulls every image.

### 5. A Version records a signature's hash, not the image

The timeline stores `sha256:` and 12 hex digits for each side of a Signature, on the
document and in child rows; the desk says Signed, Removed or Changed. Data Import does
not take the field.

**Rationale:** two copies of a 64 KiB image per change would make the Version table the
largest in the site for no reader's benefit — nobody compares two signatures pixel by
pixel in a timeline. A spreadsheet cell cannot carry a drawn signature, so the column is
shown as not importable, with the reason, rather than failing per row.

### 6. Geolocation stores one canonical FeatureCollection, as a map

`internal/geo.Normalize` accepts a FeatureCollection, a Feature or a bare Point,
MultiPoint, LineString or Polygon, and returns one canonical collection: positions
`[lon, lat]` rounded to 7 decimals (about 1 cm), altitude and `properties` dropped, rings
closed, a ring of at least 4 positions and a line of at least 2, at most 500 features and
64 KiB, an empty collection as null. The cast returns it as a `map[string]any` decoded
from its own JSON, and `sameFieldValue` compares Geolocations as JSON.

**Rationale:** version stability. A map decoded from JSON has exactly the types pgx
returns for a `jsonb` column (`float64`, `[]any`, `map[string]any`), so the value read back
equals the value cast, a document saved untouched records no Version, and a read-only or
submitted field compares equal to itself — the regression the `JSON` type has. Rounding
makes a coordinate that came through a float and back land on the same digits. Returning
a map, not a string, is also what lets a hook read `doc.area.features.length`.
Properties are dropped because nothing edits them and they would otherwise be arbitrary
JSON riding along in a validated field; a Frappe circle arrives as its centre point.
The desk's `lib/geo.ts` mirrors the normalizer, and both run
`internal/geo/testdata/cases.json`, so the map commits what the server stores.

### 7. Print shows a summary, never a map

A Geolocation prints as a key/value: a single point as `lat, lon`, anything else as
translated counts ("2 points, 1 polygon"). Lists, grid exports and history say it the
same way; `/api/export` carries the GeoJSON.

**Rationale:** a map in a PDF means the renderer fetching tiles from a third-party server
while it runs — a server-side request to wherever the tile URL points (SSRF), made on
every print, and a dependency of the print on that server being up. The same reasoning
already keeps external images out of rich text.

### 8. Map tiles come from the environment, OpenStreetMap by default

`DDCORE_MAP_TILE_URL` (a Leaflet template with `{z}`, `{x}`, `{y}`) and
`DDCORE_MAP_ATTRIBUTION` are read by `internal/config`, env only, and reach the desk in
`/api/boot` as `site.map`. With no setting the map uses OpenStreetMap's own tile server
and its credit; with another URL the credit is whatever the operator sets, or none. The
desk sanitizes the attribution before Leaflet writes it as markup. Only the browser
fetches tiles.

**Rationale:** the tile provider, and the key a paid one puts in the URL, differ per
deployment — development on the public servers, production on a contract — which is the
project's rule for `.env` over `ddcore.json`. OSM's usage policy rules out heavy use of
its servers, so the default is documented as a development default, not a production
one. Leaflet is loaded on demand, like Tiptap, and draws with `circleMarker`: the default
marker icon is an image Leaflet resolves relative to its CSS, which a bundler moves.

## Known limitations

- Autocomplete suggestions are not enforced, and a child-table column's options cannot be
  changed with `setDfProperty`.
- Barcode has three symbologies, and camera scanning needs `BarcodeDetector` (Chromium on
  desktop and Android).
- A Signature is at most 64 KiB, is not importable, and its Versions hold a hash marker, not
  the image.
- Geolocation has no vertex editing (delete and redraw), no properties, circles or
  altitude, and no map in print. The default OpenStreetMap tiles are not for production
  traffic.
- There is still no `Phone` type; input masks cover it.
