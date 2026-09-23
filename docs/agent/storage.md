# File storage

Uploaded files — `Attach` field values, attachments on a document, mail
attachments — are `File` documents. The row holds the metadata; the bytes live
in a **store**, chosen per deployment:

| Backend | Where the bytes are | Download |
| --- | --- | --- |
| `local` (default) | `<dataDir>/files/public` and `<dataDir>/files/private` | streamed by the server |
| `s3` | an S3-compatible bucket: AWS S3, Cloudflare R2, MinIO, Backblaze B2 | the server checks permission, then redirects to a short-lived presigned URL |

App code never touches the store. A file's `file_url` (`/files/<name>` or
`/private/files/<name>`) is its only name, on both backends, so moving a site
between them does not change a row.

## Configuration

Storage is a deployment decision and its credentials are secrets, so it is read
from the environment (`.env` or the real environment), never from `ddcore.json`:

```sh
DDCORE_STORAGE=s3                     # default local; the name is case-insensitive
DDCORE_S3_ENDPOINT=s3.amazonaws.com   # host[:port], no scheme; this is also the default
DDCORE_S3_REGION=us-east-1            # default empty
DDCORE_S3_BUCKET=my-site-files        # required, no default
DDCORE_S3_ACCESS_KEY=...              # required, no default
DDCORE_S3_SECRET_KEY=...              # required, no default
DDCORE_S3_PREFIX=                     # default empty: several sites in one bucket. Surrounding slashes are trimmed
DDCORE_S3_USE_SSL=true                # default
DDCORE_S3_PATH_STYLE=false            # default: the client auto-detects addressing. true forces endpoint/bucket, which MinIO and most self-hosted servers need
DDCORE_S3_PRESIGN_TTL=5m              # default 5m, 1s..168h
```

An unset bucket or credential, an endpoint with a scheme, a `DDCORE_S3_PRESIGN_TTL`
that is not a duration in range, or an unknown backend stops the process at boot.
Nothing else is checked there: no connection is opened until the first request,
so a bucket that does not exist, a wrong key or an unreachable endpoint surfaces
only on the first upload or download. `ddcore doctor` does not probe the bucket
either; it prints `storage: s3 <endpoint>/<bucket>/<prefix> (presigned links
valid <ttl>)`, without the `/<prefix>` when there is none, or
`storage: local <dataDir>/files` — never the keys. The same string is
the `storage` field of `doctor --json`.

Object keys are `<prefix>/public/<name>` and `<prefix>/private/<name>`: the
same layout as the local `files/` directory.

## Access

- **Keep the bucket private.** Public files are not served from a public bucket
  URL; they go through `/files/…`, which is the server reading the store for
  anyone who asks, Guest included. Nothing there is permission-checked; only the
  file's random name keeps it obscure.
- `/private/files/…` answers only a signed-in user who may read the File (the
  document it is attached to, and the field if it is restricted — see
  `field-permissions` and `sharing`). With `s3`, only then does the server
  issue a presigned URL.
- A presigned URL works for anyone holding it until it expires. Keep
  `DDCORE_S3_PRESIGN_TTL` short; the redirect is sent with `Cache-Control: no-store`.
- Only `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp` and `.pdf` are served inline —
  another image type such as `.bmp` or `.avif` downloads. Everything else is a
  download with `application/octet-stream`, whatever content type the uploader
  claimed. With `s3` this is enforced through the presigned URL's response
  overrides; `X-Content-Type-Options: nosniff` and the CSP sandbox are on the
  redirect only, so the bucket's own response carries neither.
- A bare `/files/` lists nothing.

### Uploading onto a document

`POST /api/upload` with `doctype` and `doc_id` hangs the file on that document, which lets
everyone who reads the document read the file. So it needs **write** on the document. With
`doctype` and no `doc_id`, or with an id that does not exist yet (a document being created),
it needs `create` or `write` on the DocType, and the file is stored detached. When the
document is saved, each `Attach`/`Attach Image` value, child rows included, that names a
detached file **the saving user uploaded** attaches it to the document, under the field, or
the Table field for a row. From then on the file follows the document. Before this, a file
picked on a new form stayed readable only by its uploader and System Manager. A Website User
uploads only into an editable attachment field of a portal page, and always privately
(see `portal`).

## Deletion

Deleting a `File` deletes its bytes, and deleting a document deletes the rows and
bytes of the files naming it in `attached_to_doctype`/`attached_to_id`. The
bytes are removed **after the transaction commits**, so a rolled-back delete
keeps them. Removal is best effort: if the store refuses, the delete still
succeeds and the server logs `file bytes left behind after delete` with the
`file_url`.

An upload whose `File` row cannot be inserted removes the bytes it just wrote.

Deleting the row is the only thing that removes bytes. These do not:

- replacing or clearing an `Attach` value — the old `File` row and its bytes stay;
- editing a `File`'s `file_url` or `is_private` — nothing moves in the store, so
  the row ends up naming bytes that are not there and the old bytes no row;
- deleting a document an attachment reaches only through an `Attach` value: an
  upload that carried no `doctype`/`doc_id` has no `attached_to_*` to cascade from.

Role `All` holds `create` and `delete` on `File` with `ifOwner`, so any user can
remove orphaned bytes by inserting a row with that `file_url` and deleting it
again — never bytes a `File` row still names, because `file_url` is `unique`.

Not covered: bytes already orphaned, with no row left to delete — including files
orphaned by direct SQL on `tab_file` — quotas, and retention windows (PRD-05,
deferred).

## Moving an existing site to S3

Copy the directory, then switch the variable:

```sh
mc mirror data/files myremote/my-site-files/<prefix>
```

Then set `DDCORE_STORAGE=s3` and restart. A backup round trip does the same work:
`ddcore restore` writes every `files/*` entry of the archive into whatever store
is configured, so an archive taken on `local` and restored with `DDCORE_STORAGE=s3`
moves the site's bytes into the bucket. A backup collects the files through the
store's own listing, and `ddcore backup --to s3` falls back to these `DDCORE_S3_*`
settings for any `DDCORE_BACKUP_S3_*` left unset — see [backup.md](backup.md).

`ddcore export --attachments` reports any File whose bytes the new store cannot
find as `missing`.

## Local S3 for development

`docker-compose.yml` has an opt-in MinIO with a `ddcore` bucket:

```sh
docker compose --profile s3 up -d
# console: http://localhost:9001  (ddcore / ddcore-secret)
```

```sh
DDCORE_STORAGE=s3
DDCORE_S3_ENDPOINT=localhost:9000
DDCORE_S3_BUCKET=ddcore
DDCORE_S3_ACCESS_KEY=ddcore
DDCORE_S3_SECRET_KEY=ddcore-secret
DDCORE_S3_USE_SSL=false
DDCORE_S3_PATH_STYLE=true
```

The storage and API tests run against it when `DDCORE_TEST_S3_ENDPOINT=localhost:9000`
is set; without it the S3 test is skipped and the API tests use the local store.
