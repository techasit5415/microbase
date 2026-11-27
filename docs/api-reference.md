# REST API Reference

Complete HTTP endpoint reference for **Microbase** (`v0.10.x`).

- **Base URL**: `http://localhost:3000` (configurable via `PORT`)
- **OpenAPI Spec**: `GET /api/openapi.json` (dynamically reflects your current collections)
- **Authentication Header**: `Authorization: Bearer <jwt_token>`
- **Response Envelope**:
  - Success responses return JSON directly (or `{ "data": ... }` depending on endpoint).
  - Error responses return:
    ```json
    {
      "error": {
        "code": "INVALID_INPUT",
        "message": "Human-readable error description"
      }
    }
    ```

### Auth Legend

| Badge | Meaning |
|---|---|
| `Public` | No token required |
| `Auth` | Valid Bearer JWT (`users`, custom auth collection, or `_superusers`) |
| `Rule` | Governed by the collection's API Rules (`list`, `view`, `create`, `update`, `delete`); `_superusers` always bypass rules |
| `Admin` | Requires a `_superusers` Bearer JWT (`role = "superuser"`) |

---

## 1. Health & Meta

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/health` | `Public` | Server health, version, and cache backend (`memory` or `redis`) |
| `GET` | `/api/openapi.json` | `Public` | Dynamic OpenAPI 3.0 specification (hides locked collections from non-superusers) |
| `GET` | `/api/settings/app` | `Public` | Public UI/branding metadata (`superuser_ips` and `trusted_proxy` are only returned to superusers) |
| `PUT` | `/api/settings/app` | `Admin` | Update application settings |

---

## 2. Superusers (`_superusers`)

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/superusers/status` | `Public` | Returns `{ "has_superusers": boolean }` |
| `POST` | `/api/superusers/setup` | `Public` | First-run superuser creation using the 8-digit terminal Setup OTP |
| `POST` | `/api/superusers/login` | `Public` | Authenticate a superuser (`email`, `password`) → returns JWT |
| `POST` | `/api/superusers` | `Admin` | Create an additional superuser |
| `PATCH` | `/api/superusers/:id` | `Admin` | Update superuser email or password |
| `DELETE` | `/api/superusers/:id` | `Admin` | Delete a superuser (cannot delete the last remaining superuser) |

---

## 3. Authentication (`users` & Auth Flows)

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/auth/methods` | `Public` | Lists enabled auth methods (password, OTP, MFA, OAuth2 providers) |
| `POST` | `/api/auth/register` | `Public` | Register a new account in `"users"` |
| `POST` | `/api/auth/login` | `Public` | Login with email + password → returns `{ token, user }` |
| `GET` | `/api/auth/me` | `Auth` | Get the currently authenticated user profile |
| `POST` | `/api/auth/refresh` | `Auth` | Issue a fresh JWT for the current session |
| `POST` | `/api/auth/request-verification` | `Public` | Send verification email (`{ "email": "..." }`) |
| `POST` | `/api/auth/confirm-verification` | `Public` | Confirm email verification token |
| `POST` | `/api/auth/request-password-reset` | `Public` | Send password reset email |
| `POST` | `/api/auth/confirm-password-reset` | `Public` | Complete password reset (`token`, `password`, `password_confirm`) |
| `POST` | `/api/auth/request-email-change` | `Auth` | Request changing the authenticated user's email |
| `POST` | `/api/auth/confirm-email-change` | `Public` | Confirm email change token |
| `POST` | `/api/auth/request-otp` | `Public` | Send a 6-digit login OTP to user's email |
| `POST` | `/api/auth/auth-with-otp` | `Public` | Exchange `{ "otp_id", "code" }` for a JWT |
| `GET` | `/api/settings/auth-options` | `Admin` | Get OTP / MFA / token duration settings |
| `PUT` | `/api/settings/auth-options` | `Admin` | Update OTP / MFA / token duration settings |

### Users Management (`/api/users`)

Supports both `BIGSERIAL` integer IDs and PocketBase-compatible 15-char `TEXT` IDs.

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/users` | `Rule` | List users (supports `?page`, `?per_page` max 500, `?sort`, `?search`; respects `email_visibility`) |
| `GET` | `/api/users/:id` | `Rule` | Fetch a user by ID (hides `email` unless `email_visibility=true`, owner, or superuser) |
| `PATCH` | `/api/users/:id` | `Rule` | Update user fields (`email`, `password`, custom fields; `role`/`verified` require admin/superuser; changing `email` resets `verified=false` for regular users) |
| `DELETE` | `/api/users/:id` | `Rule` | Delete a user by ID |
| `POST` | `/api/users/:id/impersonate` | `Admin` | Generate a short-lived impersonation JWT (`{ "duration": 3600 }`) for any user ID |

### OAuth2 / OIDC

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/auth/oauth/:provider` | `Public` | Redirect to provider authorization URL (`google`, `discord`, custom OIDC) |
| `GET` | `/api/auth/oauth/:provider/callback` | `Public` | OAuth2 callback handler |
| `POST` | `/api/auth/oauth/exchange` | `Public` | Exchange one-time OAuth code for Microbase JWT |
| `GET` | `/api/oauth/providers` | `Admin` | List configured OAuth2/OIDC providers |
| `PUT` | `/api/oauth/providers/:name` | `Admin` | Create or update an OAuth2/OIDC provider config |

---

## 4. Collections & Schema Portability

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/collections` | `Admin` | List all collections with schema, indexes, and API rules |
| `POST` | `/api/collections` | `Admin` | Create a `base`, `view`, or `auth` collection |
| `GET` | `/api/collections/:name` | `Admin` | Get a single collection definition |
| `PATCH` | `/api/collections/:name` | `Admin` | Alter collection schema, rules, indexes, or view query |
| `DELETE` | `/api/collections/:name` | `Admin` | Drop a collection and its PostgreSQL table/view |
| `POST` | `/api/collections/:name/truncate` | `Admin` | Truncate all records from a collection |
| `POST` | `/api/collections/:name/duplicate` | `Admin` | Clone a collection schema under a new name |
| `GET` | `/api/schema/export` | `Admin` | Export all collections, fields, indexes, and rules as JSON |
| `POST` | `/api/schema/import` | `Admin` | Import Microbase schema JSON **or** PocketBase `pb_schema.json` array (max 10 MB) |

---

## 5. Records, Vector Search & Custom Auth Collections

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/collections/:name/records` | `Rule` | List records (`?page`, `?perPage`/`?per_page`, `?sort`, `?filter`, `?search`, `?expand`, `?fields`, `?skipTotal`) |
| `POST` | `/api/collections/:name/records` | `Rule` | Create a record (`application/json` or `multipart/form-data`) |
| `GET` | `/api/collections/:name/records/:id` | `Rule` | Fetch a single record (`?expand`, `?fields`) |
| `PATCH` | `/api/collections/:name/records/:id` | `Rule` | Partially update a record |
| `DELETE` | `/api/collections/:name/records/:id` | `Rule` | Delete a record |
| `POST` | `/api/collections/:name/search/vector` | `Rule` | Semantic similarity search via `pgvector` (`field`, `vector`, `limit`) |
| `POST` | `/api/collections/:name/register` | `Public` | Register an account in a custom `auth` collection |
| `POST` | `/api/collections/:name/auth-with-password` | `Public` | Login to a custom `auth` collection (`identity`/`email`, `password`) |
| `POST` | `/api/collections/:name/auth-refresh` | `Auth` | Refresh JWT for a custom `auth` collection |

### List Query Parameters (`GET /api/collections/:name/records`)

| Query Param | Type | Description |
|---|---|---|
| `page` | `Number` | Page number (offset) of the paginated list (default `1`). |
| `perPage` / `per_page` | `Number` | Maximum returned records per page (default `20`, max `100`). |
| `sort` | `String` | Comma-separated sort fields (`-` DESC / `+` ASC), e.g. `?sort=-created,id`. Supports `@random` (`ORDER BY RANDOM()`) and `created`/`updated` aliases (`created_at`/`updated_at`). |
| `filter` | `String` | PocketBase-compatible filter expression (see [API Rules & Filters](api-rules.md)). |
| `search` | `String` | Full-text (English FTS) + Thai `ILIKE` search across all text columns. |
| `expand` | `String` | Auto-expand relation fields into `record.expand`, e.g. `?expand=author,user_type` (enforces target collection `view` rules). |
| `fields` | `String` | Comma-separated field projection, e.g. `?fields=id,title,created_at`. |
| `skipTotal` / `skip_total` | `Boolean` | If `1` or `true`, skips the `COUNT(*)` query and returns `total_items: -1` for faster pagination/search. |

> **JS SDK Auto-Auth**: When using `@techasit5415/microbase-sdk`, any token stored in `mb.authStore.token` is attached automatically as `Authorization: Bearer <token>` on every request — you do not need to pass the header manually.

### Common List Query Parameters

| Parameter | Example | Description |
|---|---|---|
| `page` | `page=2` | 1-indexed page number (default `1`) |
| `per_page` / `perPage` | `per_page=30` | Items per page (`1..500`, default `30`) |
| `sort` | `sort=-created_at,email` | Comma-separated fields; prefix `-` for `DESC` |
| `filter` | `filter=(status='open' && views>10)` | PocketBase-style filter expression compiled to parameterized SQL |
| `expand` | `expand=user,gpu_model` | Expand single or multi-relation foreign keys into `record.expand` |
| `fields` | `fields=id,title,created_at` | Restrict returned columns |

---

## 6. Realtime, Files & Batch

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/realtime` | `OptionalAuth` | Server-Sent Events (SSE) stream (`?collection=posts`; only endpoint accepting `?token=` query param) |
| `POST` | `/api/files` | `Auth` | Upload file to local disk or S3/R2/MinIO (max 20 MB) |
| `POST` | `/api/files/token` | `Auth` | Mint a short-lived (2-minute) signed token for protected file downloads |
| `GET` | `/api/files/:filename` | `Public` / `Token` | Serve uploaded file (pass `?protected=1` with `?token=` or Bearer auth; preset `?thumb=WxH` sizes) |
| `POST` | `/api/batch` | `OptionalAuth` | Execute up to 50 sub-requests in a single HTTP round-trip (preserves client IP, blocks recursive `/api/batch`) |

---

## 7. Logs & Observability

Static Admin UI paths (`/_`, `/_/*`, `/favicon.ico`, `/api/logs`) are automatically excluded from HTTP request logging.

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/logs` | `Admin` | Paginated request logs (`?page=1&per_page=30&search=...`) |
| `DELETE` | `/api/logs` | `Admin` | Permanently delete all persisted logs |
| `GET` | `/api/settings/logs` | `Admin` | Get current log settings |
| `PUT` | `/api/settings/logs` | `Admin` | Update log settings (`max_days`, `min_level`, `max_bytes`, `log_ip`, `log_auth_id`) |
| `GET` | `/api/system/:table` | `Admin` | Inspect internal system tables (`_superusers`, `_logs`, etc.) |

### Example: `GET /api/logs?page=1&per_page=30&search=WARN`

```json
{
  "page": 1,
  "per_page": 30,
  "total_items": 412,
  "total_pages": 14,
  "items": [
    {
      "id": 689,
      "level": 4,
      "timestamp": "2026-11-26T16:56:19Z",
      "method": "GET",
      "path": "/api/collections/private_notes/records",
      "url": "/api/collections/private_notes/records?page=1&per_page=20",
      "status": 403,
      "code": "403",
      "message": "Forbidden",
      "latency_ms": 2,
      "exec_time_ms": 1.842,
      "ip": "127.0.0.1",
      "remote_ip": "127.0.0.1",
      "auth": "users",
      "auth_role": "user",
      "auth_email": "alice@example.com",
      "auth_id": "a1b2c3d4e5f6g7h"
    }
  ]
}
```

### Example: `PUT /api/settings/logs`

```json
{
  "max_days": 7,
  "min_level": 0,
  "max_bytes": 16384,
  "log_ip": true,
  "log_auth_id": true
}
```

- `max_days`: `0` disables log persistence and deletes all existing logs; `1..365` sets retention days.
- `min_level`: `-4` (`DEBUG`), `0` (`INFO`), `4` (`WARN`), `8` (`ERROR`). Requests below `min_level` are not persisted.

---

## 8. Operations, Settings, Backups, Crons, Webhooks & SQL

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` / `PUT` | `/api/settings/database` | `Admin` | Inspect or update PostgreSQL connection config |
| `POST` | `/api/settings/database/test` | `Admin` | Test a PostgreSQL connection string |
| `GET` / `PUT` | `/api/settings/redis` | `Admin` | Get (password masked as `REDACTED`) or update Redis cache URL |
| `GET` / `PUT` | `/api/settings/s3` | `Admin` | Get or update S3 / R2 / MinIO object storage settings |
| `GET` / `PUT` | `/api/settings/mail` | `Admin` | Get or update SMTP mail server settings |
| `POST` | `/api/settings/mail/test` | `Admin` | Send a test email (`{ "to": "you@example.com" }`) |
| `GET` / `PUT` | `/api/settings/ratelimits` | `Admin` | Toggle live rate limiting (`enabled`) and configure per-prefix rules |
| `GET` | `/api/backups` | `Admin` | List PostgreSQL backup archives |
| `POST` | `/api/backups` | `Admin` | Create a new `pg_dump` backup archive |
| `GET` | `/api/backups/:name` | `Admin` | Download backup archive (send `Authorization: Bearer` header) |
| `POST` | `/api/backups/upload` | `Admin` | Upload a backup archive |
| `POST` | `/api/backups/:name/restore` | `Admin` | Restore database from a backup archive |
| `DELETE` | `/api/backups/:name` | `Admin` | Delete a backup archive |
| `GET` | `/api/crons` | `Admin` | List scheduled cron jobs |
| `POST` | `/api/crons` | `Admin` | Create a cron job |
| `PATCH` | `/api/crons/:id` | `Admin` | Update a cron job |
| `DELETE` | `/api/crons/:id` | `Admin` | Delete a cron job |
| `POST` | `/api/crons/:id/run` | `Admin` | Trigger a cron job immediately |
| `GET` | `/api/webhooks` | `Admin` | List configured webhooks (`generic`, `discord`, `fcm`) |
| `POST` | `/api/webhooks` | `Admin` | Create a webhook |
| `PATCH` | `/api/webhooks/:id` | `Admin` | Enable or disable a webhook |
| `POST` | `/api/webhooks/:id/test` | `Admin` | Send a test payload to a webhook |
| `DELETE` | `/api/webhooks/:id` | `Admin` | Delete a webhook |
| `POST` | `/api/sql` | `Admin` | Execute raw SQL against PostgreSQL (`{ "query": "SELECT ..." }`) |
