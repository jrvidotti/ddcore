package config

// EnvExample is the committed record of which environment variables this
// framework reads. `ddcore init` writes it, and the repository keeps a copy at
// .env.example — envexample_test.go fails if the two drift apart, because the
// copy people read is the one in the repository, not this string.
const EnvExample = `# ddcore — the environment this site runs in.
#
# Copy to ` + "`" + `.env` + "`" + ` (gitignored) and fill in. Everything here is per-deployment or
# secret; the site's own decisions — currency, precision, timezone, apps, access
# policy — live in the committed ddcore.json instead.
#
# Precedence: a variable already set in the real environment wins over this
# file, which wins over ddcore.json. That is what lets Railway, Docker or a
# systemd unit override a file baked into the image without editing it.

# --- database and server -----------------------------------------------------
# DDCORE_DSN=postgres://ddcore:ddcore@localhost:5455/ddcore?sslmode=disable
# DATABASE_URL is read too, and is what Railway injects on its own.
# DDCORE_PORT=8090

# --- public address ----------------------------------------------------------
# The base a recovery or invitation link is built from. Leave it out and the
# server falls back to http://localhost:<port> and says so at boot — a link
# pointing at the wrong host is worse than no mail at all.
# DDCORE_URL=https://erp.example.com

# Only turn this on when a proxy you control really sets X-Forwarded-For.
# With it off the server uses the socket peer, which no client can forge.
# DDCORE_TRUST_PROXY=false

# --- sign-in screen ----------------------------------------------------------
# A notice above the sign-in form, and optionally a demo account with a button
# that fills the form in. Anyone who opens the site sees all three, so this is
# for a public demo — never a real password. \n in the notice breaks the line.
# DDCORE_LOGIN_NOTICE=Public demo — data resets every 6 hours.
# DDCORE_LOGIN_DEMO_USER=visitor@example.com
# DDCORE_LOGIN_DEMO_PASSWORD=

# --- logging -----------------------------------------------------------------
# The log's shape is a property of where the process runs, not of the site: a
# terminal reads text, a platform that ships stdout to a collector needs
# objects it can index and group by request id. Unset, the process decides by
# looking at where the log goes — a terminal gets text, anything else JSON —
# so a deployment needs no setting here. Accepts json or text.
# DDCORE_LOG_FORMAT=json
# Anything non-empty raises the level to debug, which also logs every static
# asset the desk requests.
# DDCORE_DEBUG=

# --- mail --------------------------------------------------------------------
# How a message leaves the site: the framework's own recovery and invitation
# links, and whatever an app sends with ddcore.sendMail.
#   log    — write it to the log (the default, and what development uses)
#   smtp   — hand it to a relay
#   method — hand it to an app function, e.g. an HTTP e-mail API
# DDCORE_MAIL_TRANSPORT=log
# DDCORE_MAIL_FROM=ddcore <no-reply@example.com>

# For DDCORE_MAIL_TRANSPORT=method
# DDCORE_MAIL_METHOD=myapp.services.mail.send

# For DDCORE_MAIL_TRANSPORT=smtp
# DDCORE_SMTP_HOST=smtp.example.com
# DDCORE_SMTP_PORT=587
# DDCORE_SMTP_TLS=starttls          # starttls | tls | none
# DDCORE_SMTP_USERNAME=
# DDCORE_SMTP_PASSWORD=
# DDCORE_MAIL_DEBUG=                # in dev mode, redirect all outgoing mail to this address

# Total bytes of attachments allowed on one message. Checked when the message is
# queued, so an oversized attachment fails where someone can still see it.
# DDCORE_MAIL_MAX_ATTACHMENT=10485760

# --- file storage ------------------------------------------------------------
# Where uploaded files keep their bytes.
#   local — <dataDir>/files on this machine (the default)
#   s3    — an S3-compatible bucket: AWS S3, Cloudflare R2, MinIO, Backblaze B2
# With s3, downloads are still permission-checked by the server, which then
# redirects the browser to a short-lived presigned URL. Keep the bucket private.
# DDCORE_STORAGE=local
# DDCORE_S3_ENDPOINT=s3.amazonaws.com   # host[:port], no scheme
# DDCORE_S3_REGION=us-east-1
# DDCORE_S3_BUCKET=
# DDCORE_S3_ACCESS_KEY=
# DDCORE_S3_SECRET_KEY=
# DDCORE_S3_PREFIX=                     # key prefix, to share one bucket between sites
# DDCORE_S3_USE_SSL=true
# DDCORE_S3_PATH_STYLE=false            # true for MinIO and most self-hosted servers
# DDCORE_S3_PRESIGN_TTL=5m              # how long a download link stays valid

# --- outgoing webhooks -------------------------------------------------------
# Subscriptions are Webhook documents, set up in the desk. This switch is for a
# deployment that must have no outgoing business effects — a migration
# rehearsal, a restored copy of production. While off, events queue nothing, so
# turning it back on does not release a backlog. ` + "`" + `ddcore doctor` + "`" + ` warns while it is off.
# DDCORE_WEBHOOKS=on

# The master key for encrypted Vault fields, which is where a webhook's signing
# secret is kept. Without it a Webhook cannot be saved or signed.
# DDCORE_SECRET_KEY=

# --- integration secrets -----------------------------------------------------
# Credentials an app needs to talk to something else. They live here and never
# in a column: a secret in the database is a secret in every backup, every
# export, every replica and every Version diff, and keeping it out of those is
# a list of places to remember rather than a property of the system. Rotating
# one is a redeploy, not a migration.
#
# ` + "`" + `ddcore.secret("stripe_key")` + "`" + ` reads DDCORE_SECRET_STRIPE_KEY. The prefix is
# the boundary: an app can read its own secrets and nothing else the process
# was started with. ` + "`" + `ddcore doctor` + "`" + ` lists the names it found, never the values.
# DDCORE_SECRET_STRIPE_KEY=
# DDCORE_SECRET_WHATSAPP_TOKEN=
`
