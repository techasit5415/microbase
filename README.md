# Microbase

**Microbase** is a self-hosted, single-binary Backend-as-a-Service (BaaS) inspired by PocketBase and powered by **PostgreSQL** (`pgvector`), **Go (Echo v4)**, optional **Redis**, **S3-compatible object storage**, and an embedded **SvelteKit 3 + Svelte 5** Admin Dashboard (`/_/`).

## Highlights

- **Single Binary + Docker**: Embedded Admin Dashboard (`go:embed`), multi-platform GitHub Releases, and multi-arch GHCR Docker images (`ghcr.io/techasit5415/microbase`).
- **Dynamic PostgreSQL Collections**:
  - **Base** (`CREATE TABLE`), **View** (`CREATE OR REPLACE VIEW`), and **Auth** collections (including PocketBase-compatible `"users"`).
  - **17 Field Types**: `text`, `editor`, `number`, `integer`, `float`, `boolean`, `email`, `url`, `datetime`, `autodate`, `file`, `relation` (real PostgreSQL FKs with `TEXT` & `BIGINT` support), `select`, `json`, `geopoint`, and `vector` (`pgvector`).
  - **PocketBase Schema Import**: Directly upload or paste `pb_schema.json` (JSON array) or Microbase schema JSON with automatic dependency ordering.
- **PocketBase-Style Admin UI (`/_/`)**:
  - Copyable ID pills, `True`/`False` & `select` badges, expanded relation pills with **Eye (`👁`) JSON hover preview**, `Presentable` field labels, flicker-free column sorting (`ASC`/`DESC`), full pagination, and a **Records Picker Modal**.
  - Interactive **ERD Schema Canvas** with field-to-table SVG relation lines, **SQL Console**, **Persistent Logs (`_logs`)**, **Live Rate Limiting Toggle**, **Backups & Restore (`pg_dump`)**, **Crons**, and **Webhooks**.
- **Security & Auth**:
  - Argon2id password hashing, JWT auth, Refresh, Email Verification, Password Reset, Email Change, 6-digit OTP, MFA, Superuser Impersonation, and OAuth2/OIDC (Google, Discord, Custom IAM).
  - **Row-Level API Rules**: Secure-by-default rules compiled into parameterized PostgreSQL SQL.

---

## Quick Start

### Option 1: Docker (GHCR)

```bash
docker pull ghcr.io/techasit5415/microbase:latest
```

### Option 2: Build from Source

```bash
# 1. Start PostgreSQL (with pgvector)
docker compose up -d

# 2. Configure environment variables
cp .env.example .env
# Edit .env and set DATABASE_URL and a strong JWT_SECRET (e.g. openssl rand -hex 32)

# 3. Build Frontend & Single Go Binary
cd ui && npm install && npm run build && cd ..
CGO_ENABLED=0 go build -trimpath \
  -ldflags="-s -w -X github.com/techasit5415/microbase/internal/config.Version=v0.12.2" \
  -o microbase ./cmd/server

# 4. Run Microbase
./microbase
```

On first launch (when `_superusers` is empty), Microbase prints a one-time **8-digit Setup OTP** (valid for 5 minutes) in the server terminal. Open `http://localhost:3000/_/` to create your first Superuser account.

---

## JavaScript / TypeScript SDK

```bash
npm install @techasit5415/microbase-sdk
```

---

## Documentation

| Guide | Topics Covered |
|---|---|
| **[Getting Started & Deployment](docs/getting-started.md)** | Installation, `.env` configuration, Docker, first-run Superuser setup |
| **[API Rules & Access Control](docs/api-rules.md)** | Setting `list`/`view`/`create`/`update`/`delete` rules, operators, `@request.auth.*` macros, copy-paste recipes |
| **[Collections, Fields, Querying & Realtime](docs/collections-and-records.md)** | 17 field types, `?filter`, `?sort`, `?expand`, `?fields`, `pgvector` search, SSE Realtime, Batch API |
| **[Authentication & OAuth2/OIDC](docs/authentication.md)** | Register/Login, JWT, OTP, Password Reset, Impersonation, Google/Discord/OIDC Claim Mapping |
| **[Storage, Redis, Backups & Operations](docs/storage-and-operations.md)** | S3/R2/MinIO storage, Redis cache, SMTP mail, Webhooks, Crons, Rate Limits, `pg_dump` Backups |
| **[REST API Reference](docs/api-reference.md)** | Complete endpoint table for Auth, Collections, Records, Realtime, Files, Logs, Settings, Backups, Crons & SQL |
| **[CLI, Migrations & JS Hooks](docs/cli-hooks-migrations.md)** | CLI Flags (`--no-ui`, `update`, `superuser`), `mb_migrations`, `mb_hooks` (Goja), Thumbnails, File Tokens |
| **[Admin Dashboard Guide (`/_/`)](docs/admin-ui.md)** | Bookmarkable routes (`/_/settings/:section`), UI Appearance customizer, ERD Canvas, Logs Drawer & Settings modal |
| **[Load Testing (Grafana k6)](loadtest/k6/README.md)** | Smoke, Load (50 VUs), and Stress (150 VUs) benchmark scripts |
