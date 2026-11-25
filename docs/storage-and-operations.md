# Storage, Caching, Webhooks, Mail & Operations

## File Storage (Local Disk & Hot-Swappable S3)

- **Upload**: `POST /api/files` (`multipart/form-data` with field `file`, max 20 MB).
- **Download**: `GET /api/files/:filename`
- **On-the-fly Image Thumbnails**: Append `?thumb=WxH` (e.g. `?thumb=100x100` or `?thumb=480x0`) for JPEG/PNG resizing.

### Connecting S3 / Cloudflare R2 / MinIO

Open **Settings -> Files Storage (S3 / R2 / MinIO)** (`PUT /api/settings/s3`):
- Toggle **Enabled**, fill **Endpoint URL**, **Bucket**, **Region** (`auto` for R2), **Access Key**, and **Secret Key**.
- Enable **Force path-style addressing** for MinIO or Cloudflare R2.
- Microbase tests bucket access via AWS SigV4 `HEAD Bucket` and hot-swaps storage immediately without restarting.

## Hot-Swappable Redis Cache

By default, Microbase uses an in-memory cache (`memory`) for schema caching, rate limiting, OAuth states, and OTPs.
To scale horizontally, open **Settings -> Redis Cache URL** (`PUT /api/settings/redis`) and enter `redis://host:6379/0`. Microbase verifies `PING` and hot-swaps the cache live.

## Notification Hooks (Webhooks)

Configure outbound event hooks in **Settings -> Notification Hooks** (`POST /api/webhooks`) for `create`, `update`, `delete`, or `*` events:

- **Discord Webhook**: Sends formatted rich Embeds with collection name, event, and record payload.
- **Firebase FCM (HTTP v1)**: Sends push notification payloads with `Authorization: Bearer <token>`.
- **Generic HTTP**: POSTs JSON `{ "collection", "event", "record", "timestamp" }`.

## SMTP Mail

Configure SMTP in **Settings -> Mail Settings (SMTP)** (`PUT /api/settings/mail`) and verify delivery with **Send test** (`POST /api/settings/mail/test`). Used by email verification, password reset, email change, and user OTP flows.

## Backups & Restore (`pg_dump` / `psql`)

Manage PostgreSQL SQL snapshots in **Settings -> Backups & Restore**:
- `POST /api/backups` — Create a timestamped `.sql` dump via `pg_dump`.
- `POST /api/backups/upload` — Upload an existing `.sql` backup file.
- `GET /api/backups/:name` — Download backup file.
- `POST /api/backups/:name/restore` — Restore snapshot into PostgreSQL via `psql`.
- `DELETE /api/backups/:name` — Remove backup file.

## Cron Jobs, Rate Limits & Logs

- **Cron Jobs** (`GET /api/crons`, `POST /api/crons/:id/run`): Run built-in maintenance jobs (`__mbLogsPrune__`, `__mbAutoBackup__`) on demand.
- **Per-Route Rate Limits** (`GET/PUT /api/settings/ratelimits`): Hot-swap per-path-prefix limits (e.g. `/api/auth/login` -> 20 req / 60s, `/api/` -> 120 req / 60s).
- **Persistent Logs** (`GET /api/logs`, `GET/PUT /api/settings/logs`): Stored asynchronously in PostgreSQL `_logs` with configurable retention days, visual volume chart, and full JSON preview modal.
- **SQL Console & ERD** (`POST /api/sql`, `/_/sql`): Execute PostgreSQL queries directly as a Superuser (with `password_hash` automatically masked) and inspect collection relationships visually.
- **Schema Portability**: Export all collection schemas (`GET /api/schema/export`) and import into another environment (`POST /api/schema/import`).
