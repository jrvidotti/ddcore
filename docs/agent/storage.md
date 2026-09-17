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
DDCORE_STORAGE=s3
DDCORE_S3_ENDPOINT=s3.amazonaws.com   # host[:port], no scheme
DDCORE_S3_REGION=us-east-1
DDCORE_S3_BUCKET=my-site-files
DDCORE_S3_ACCESS_KEY=...
DDCORE_S3_SECRET_KEY=...
DDCORE_S3_PREFIX=                     # optional: several sites in one bucket
DDCORE_S3_USE_SSL=true
DDCORE_S3_PATH_STYLE=false            # true for MinIO and most self-hosted servers
DDCORE_S3_PRESIGN_TTL=5m              # 1s..168h
```

A missing bucket or credential, an endpoint with a scheme, or an unknown backend
stops the process at boot. `ddcore doctor` prints the backend, bucket and
prefix — never the keys.

Object keys are `<prefix>/public/<name>` and `<prefix>/private/<name>`: the
same layout as the local `files/` directory.

## Access

- **Keep the bucket private.** Public files are not served from a public bucket
  URL; they go through `/files/…` like private ones.
- `/private/files/…` answers only a signed-in user who may read the File (the
  document it is attached to, and the field if it is restricted — see
  `field-permissions` and `sharing`). With `s3`, only then does the server
  issue a presigned URL.
- A presigned URL works for anyone holding it until it expires. Keep
  `DDCORE_S3_PRESIGN_TTL` short; the redirect is sent with `Cache-Control: no-store`.
- Images and PDFs are served inline, everything else as a download with
  `application/octet-stream`, whatever content type the uploader claimed. With
  `s3` this is enforced through the presigned URL's response overrides.
- A bare `/files/` lists nothing.

## Deletion

Deleting a `File` deletes its bytes, and deleting a document deletes its
attachments' rows and bytes. The bytes are removed **after the transaction
commits**, so a rolled-back delete keeps them. Removal is best effort: if the
store refuses, the delete still succeeds and the server logs
`file bytes left behind after delete` with the `file_url`.

An upload whose `File` row cannot be inserted removes the bytes it just wrote.

Not covered: bytes written before this behavior existed, files orphaned by
direct SQL on `tab_file`, quotas, and retention windows (PRD-05, deferred).

## Moving an existing site to S3

There is no built-in migration. Copy the directory, then switch the variable:

```sh
mc mirror data/files myremote/my-site-files/<prefix>
```

Then set `DDCORE_STORAGE=s3` and restart. `ddcore export --attachments` reports
any File whose bytes the new store cannot find as `missing`.

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
