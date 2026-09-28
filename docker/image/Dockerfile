# The official ddcore image: the release binary on Alpine, with what a site
# needs at run time and nothing it builds with. A site's own Dockerfile starts
# FROM this image at the release it pins and copies ddcore.json and apps/ in.
#
# Built by .github/workflows/release.yml from the binaries it has just
# compiled; the build context holds linux-<arch>/ddcore for each platform.
FROM alpine:3.21

ARG TARGETARCH

# tzdata for the site's timezone; postgresql17-client gives `ddcore backup`
# and `ddcore restore` pg_dump and pg_restore, which must be at least as new
# as the server's major version.
RUN apk add --no-cache ca-certificates tzdata postgresql17-client

COPY linux-${TARGETARCH}/ddcore /usr/local/bin/ddcore

WORKDIR /app

# Uploaded files and local backups. Mount a volume here, or use
# DDCORE_STORAGE=s3; the container's own filesystem does not survive a deploy.
ENV DDCORE_DATA_DIR=/data
RUN mkdir -p /data

# A platform sets $PORT (Railway does); ddcore reads it when DDCORE_PORT is unset.
ENV PORT=8080
EXPOSE 8080

# Migrates on boot (DDCORE_AUTO_MIGRATE=0 turns that off), then serves the
# desk, the API, the workers and the scheduler from one process.
CMD ["ddcore", "start"]
