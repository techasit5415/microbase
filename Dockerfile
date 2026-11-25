# Stage 1: Build SvelteKit Admin UI
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui-builder
WORKDIR /src/ui
COPY ui/package.json ui/package-lock.json ./
RUN npm install --no-audit --no-fund
COPY ui/ ./
RUN npm run build

# Stage 2: Compile static Go binary with embedded UI
FROM --platform=$BUILDPLATFORM golang:alpine AS go-builder
ENV GOTOOLCHAIN=auto
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /src/ui/build ./ui/build

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -X github.com/techasit5415/microbase/internal/config.Version=${VERSION}" \
    -o /out/microbase ./cmd/server

# Stage 3: Minimal production runtime with pg_dump/psql for Backups
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata postgresql17-client

LABEL org.opencontainers.image.title="Microbase" \
      org.opencontainers.image.description="Single-binary BaaS powered by PostgreSQL + pgvector with PocketBase-parity Admin UI" \
      org.opencontainers.image.source="https://github.com/techasit5415/microbase"

WORKDIR /app
COPY --from=go-builder /out/microbase /app/microbase

RUN mkdir -p /app/storage
VOLUME ["/app/storage"]

EXPOSE 3000
ENTRYPOINT ["/app/microbase"]
