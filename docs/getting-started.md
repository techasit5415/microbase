# Getting Started

## Prerequisites

- **PostgreSQL 15+** with the `pgvector` extension installed (included in `docker-compose.yml` via `pgvector/pgvector:pg16`).
- **Go 1.24+** (for building the server binary).
- **Node.js 22+** (only needed when rebuilding the embedded Admin UI).
- **Optional**: `pg_dump` and `psql` CLI tools on `PATH` for UI-triggered Backups & Restore.
- **Optional**: Redis server (can also be connected later at runtime via Settings).

## Environment Configuration (`.env`)

Create a `.env` file in the project root:

```env
PORT=3000
DATABASE_URL=postgres://microbase:microbase@localhost:5433/microbase?sslmode=disable
JWT_SECRET=replace-with-a-long-random-secret-at-least-32-chars
REDIS_URL=
```

| Variable | Required | Description |
|---|---|---|
| `PORT` | No | HTTP listen port (default `3000`). |
| `DATABASE_URL` | Yes | PostgreSQL connection URI. Microbase automatically runs schema migrations and enables `vector` on startup. |
| `JWT_SECRET` | Yes | Signing key for JWT access tokens. Never leave empty in production. |
| `REDIS_URL` | No | Optional initial Redis URI (`redis://localhost:6379/0`). Can be added or changed live in **Settings**. |

## Building the Single Binary

```bash
# 1. Build the static SvelteKit Admin UI into ui/build
cd ui
npm install
npm run check
npm run build
cd ..

# 2. Compile Go server with embedded UI
go build -ldflags="-X github.com/techasit5415/microbase/internal/config.Version=v0.10.32" -o microbase ./cmd/server
```

## First-Time Superuser Setup (8-Digit OTP)

When Microbase starts with an empty `_superusers` table, it generates a cryptographically random **8-digit Setup OTP** valid for **5 minutes** and logs it to the terminal:

```text
============================================================
FIRST-TIME SUPERUSER SETUP REQUIRED
Open:      http://localhost:3000/_/
Setup OTP: 51384844  (valid for 5 minutes)
============================================================
```

1. Open `http://localhost:3000/_/`.
2. Enter your Superuser email, password (min 8 chars), confirm password, and the 8-digit OTP.
3. Once the first Superuser is created, `POST /api/superusers/setup` is **permanently locked** (`403 SETUP_LOCKED`).
4. To add more Superusers later, sign in to `/_/` and open **System -> `_superusers` -> `+ New record`**.

## Admin Dashboard Navigation (`/_/`)

- **Collections (`/_/collections`)**: Manage `users`, custom Base/View/Auth collections, records, indexes, API rules, and API Preview.
- **Logs (`/_/logs`)**: Inspect persisted HTTP request logs (`_logs`), view the request volume chart, configure retention days, and click any row for full JSON details.
- **SQL Console & ERD (`/_/sql`)**: Run raw PostgreSQL queries (`Ctrl+Enter`) as a Superuser and inspect your visual ERD schema diagram.
- **Settings (`/_/settings`)**: Configure Redis hot-swap, S3/R2/MinIO file storage, SMTP Mail, Discord/FCM Webhooks, Backups & Restore, Cron Jobs, Per-Route Rate Limits, and Schema Export/Import.
