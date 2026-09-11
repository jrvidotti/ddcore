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

# --- mail --------------------------------------------------------------------
# How a password recovery or invitation link leaves the site.
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
`
