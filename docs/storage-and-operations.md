# Storage, Caching, Logs, Rate Limits, Mail & Operations

## File Storage (Local Disk & Hot-Swappable S3)

- **Upload**: `POST /api/files` (`multipart/form-data` with field `file`, max 20 MB).
- **Download**: `GET /api/files/:filename`
- **On-the-fly Image Thumbnails**: Append `?thumb=WxH` (e.g. `?thumb=100x100` or `?thumb=480x0`) for JPEG/PNG resizing.

### Connecting S3 / Cloudflare R2 / MinIO

Open **Settings → Files storage** (`/_/settings/storage`, `PUT /api/settings/s3`):
- Toggle **Enabled**, fill **Endpoint URL**, **Bucket**, **Region** (`auto` for Cloudflare R2), **Access Key**, and **Secret Key**.
- Enable **Force path-style addressing** for MinIO or Cloudflare R2.
- Microbase validates bucket access via AWS SigV4 `HEAD Bucket` and hot-swaps storage immediately without restarting.

---

## Hot-Swappable Redis Cache

By default, Microbase uses an in-memory cache (`memory`) for schema caching, rate limiting, OAuth states, and OTPs.
To scale horizontally, open **Settings → Database & Cache** (`/_/settings/database`, `PUT /api/settings/redis`) and set `redis://host:6379/0`. Microbase verifies `PING` and hot-swaps the cache live.

---

## Persistent Logs & Logs Settings

HTTP request logs are persisted asynchronously to PostgreSQL `_logs` and viewed at `/_/logs` (`GET /api/logs`).

- **Automatic Noise Filtering**: Static Admin UI paths (`/_`, `/_/*`, `/favicon.ico`) and `/api/logs` itself are excluded from logging.
- **Auth Identity Tracking**: Authenticated API requests automatically record `auth` (`_superusers`, `users`, or custom auth collection), `auth_email`, and `auth_id`. Unauthenticated requests store empty `auth` (displayed as `—` / `N/A`).
- **Log Levels**:
  - `-4` (`DEBUG`)
  - `0` (`INFO` — HTTP 2xx/3xx)
  - `4` (`WARN` — HTTP 4xx)
  - `8` (`ERROR` — HTTP 5xx)
- **Server-Side Pagination & Search**: `GET /api/logs?page=1&per_page=30&search=WARN` searches across all persisted logs in PostgreSQL by level, method, URL, auth collection, email, IP, status, and user-agent.
- **Logs Settings (`GET/PUT /api/settings/logs`)**:
  - `max_days` (`0..365`): Set to `0` to delete all logs and disable logs persistence.
  - `min_level` (`-8..12`): Requests with level below `min_level` are ignored.
  - `max_bytes` (`0..1048576`): Per-field cap; `0` defaults to `~16KB` (`16384` bytes).
  - `log_ip` (`bool`): Toggle recording `remoteIP` and `userIP`.
  - `log_auth_id` (`bool`): Toggle recording `auth_id`.
- **Clear All Logs**: `DELETE /api/logs` (Admin only) or click the trash icon in the **Logs settings** modal.

---

## Live Rate Limiting

Configure per-path-prefix rate limits at **Settings → Application** (`/_/settings/application`, `GET/PUT /api/settings/ratelimits`):

```json
{
  "enabled": true,
  "rules": [
    { "label": "Login", "prefix": "/api/auth/login", "max_requests": 20, "duration_seconds": 60 },
    { "label": "Global API", "prefix": "/api/", "max_requests": 120, "duration_seconds": 60 }
  ]
}
```

- Toggle `enabled: false` live (for example, during load testing or bulk imports) without restarting the server.
- Changes persist to `_settings` (`rate_limit_rules`) and are reloaded automatically on startup.
- `/api/settings/ratelimits` is always exempt from rate limiting so superusers can never lock themselves out.

---

## Notification Hooks (Webhooks)

Configure outbound event hooks in **Settings → Webhooks** (`/_/settings/webhooks`, `POST /api/webhooks`) for `create`, `update`, `delete`, or `*` events:

- **Discord Webhook**: Sends formatted rich Embeds with collection name, event, and record payload.
- **Firebase FCM (HTTP v1)**: Sends push notification payloads with `Authorization: Bearer <token>`.
- **Generic HTTP**: POSTs JSON `{ "collection", "event", "record", "timestamp" }` with optional custom headers, HMAC signing secret, and configurable retry count (`max_retries`).
- Use **Test** (`POST /api/webhooks/:id/test`) to verify delivery immediately.

---

## SMTP Mail

Configure SMTP in **Settings → Mail settings** (`/_/settings/mail`, `PUT /api/settings/mail`) and verify delivery with **Send test** (`POST /api/settings/mail/test`). Used by email verification, password reset, email change, and 6-digit login OTP flows.

---

## Backups & Restore (`pg_dump` / `psql`)

Manage PostgreSQL SQL snapshots in **Settings → Backups** (`/_/settings/backups`):
- `POST /api/backups` — Create a timestamped `.sql` dump via `pg_dump`.
- `POST /api/backups/upload` — Upload an existing `.sql` backup file.
- `GET /api/backups/:name` — Download backup file (requires `Authorization: Bearer <superuser_token>`).
- `POST /api/backups/:name/restore` — Reset the `public` schema and restore the snapshot via `psql -v ON_ERROR_STOP=1`.
- `DELETE /api/backups/:name` — Remove backup file.

> **Prerequisites**: `pg_dump` and `psql` must be available on `PATH`. If the backup was created from a database with `pgvector` enabled, the target PostgreSQL instance must also have `pgvector` installed.

### Troubleshooting: `ERROR: extension "vector" is not available`

When restoring a `.sql` backup created on a server with `pgvector` into a target PostgreSQL server that does not have `pgvector` installed, `psql` stops with:

```text
psql restore failed: exit status 3 (... ERROR: extension "vector" is not available HINT: The extension must first be installed on the system where PostgreSQL is running.)
```

**Option 1 — Install `pgvector` on the target PostgreSQL server (Recommended):**

- **Docker / Docker Compose**: Switch the PostgreSQL image from `postgres:<ver>` to `pgvector/pgvector:pg<ver>` (match your existing PostgreSQL major version, e.g. `pg16` or `pg17`):
  ```yaml
  services:
    postgres:
      image: pgvector/pgvector:pg17
  ```
  Then recreate the container (`docker compose up -d`) and run Restore again.

- **Ubuntu / Debian (VM, LXC, or bare-metal)**: Check your installed PostgreSQL major version (`psql --version` or `pg_lsclusters`) and install the matching package:
  ```bash
  sudo apt update
  sudo apt install -y postgresql-16-pgvector   # replace 16 with your PostgreSQL major version (e.g. 15, 16, 17)
  ```
  *(Restart PostgreSQL with `sudo systemctl restart postgresql` if the extension is not immediately detected, then retry Restore.)*

- **macOS (Homebrew)**:
  ```bash
  brew install pgvector
  ```

- **Windows**: Run PostgreSQL via Docker (`pgvector/pgvector:pg17`) or install the `pgvector` binaries matching your PostgreSQL major version into PostgreSQL's `lib/` and `share/extension/` directories.

**Option 2 — Remove `vector` extension lines from the `.sql` file (Only if no `vector` columns are used):**

If your collections do **not** use any `vector` field types and `pgvector` was only enabled by default during backup:
1. Open the `.sql` backup file and remove the `CREATE EXTENSION IF NOT EXISTS vector ...;` and `COMMENT ON EXTENSION vector ...;` lines.
2. Re-upload the edited `.sql` file in **Settings → Backups** and click **Restore**.
*(If any table contains a `vector(...)` column, restore will fail at `CREATE TABLE` — use Option 1 instead.)*

---

## Cron Jobs

Manage scheduled maintenance jobs in **Settings → Crons** (`/_/settings/crons`):
- `GET /api/crons` — List jobs (including built-in `__mbLogsPrune__` and `__mbAutoBackup__`).
- `POST /api/crons` / `PATCH /api/crons/:id` / `DELETE /api/crons/:id` — Create, edit, or delete schedules.
- `POST /api/crons/:id/run` — Trigger any job immediately on demand.

---

## Schema Portability & PocketBase Migration

- **Export**: `GET /api/schema/export` (or `/_/settings/export`) downloads all collections, fields, indexes, and API rules as JSON.
- **Import**: `POST /api/schema/import` (or `/_/settings/import`) accepts both:
  - Native Microbase schema JSON (`{ "version": "1.0", "collections": [...] }`)
  - PocketBase `pb_schema.json` (JSON array `[...]`) — automatically skips PocketBase system collections/fields, supports mixed-case `A-Z` identifiers, supports `TEXT` & `BIGINT` relation foreign keys, and topologically orders table creation by FK dependencies.
