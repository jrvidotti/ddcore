# syntax=docker/dockerfile:1
# Official base Dockerfile for the ddcore framework.
# Published image: ghcr.io/jrvidotti/ddcore

# Stage 1: Build the Desk SPA (SvelteKit)
FROM --platform=$BUILDPLATFORM node:22-alpine AS desk-build
WORKDIR /src/desk
COPY desk/package*.json ./
RUN npm ci --silent
COPY desk/ ./
RUN npm run build

# Stage 2: Compile the static Go binary using native cross-compilation
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go-build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=desk-build /src/desk/build ./desk/build

ARG TARGETOS TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags="-s -w -X github.com/jrvidotti/ddcore/internal/engine.Version=${VERSION}" \
    -o /bin/ddcore ./cmd/ddcore

# Stage 3: Minimal runner image
FROM alpine:3.21 AS runner
RUN apk add --no-cache ca-certificates tzdata
COPY --from=go-build /bin/ddcore /usr/local/bin/ddcore

ENTRYPOINT ["/usr/local/bin/ddcore"]
CMD ["help"]
