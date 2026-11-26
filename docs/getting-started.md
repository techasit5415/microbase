# Getting Started & Deployment

## Prerequisites

- **PostgreSQL 15+** with the `pgvector` extension installed (included in `docker-compose.yml` via `pgvector/pgvector:pg16`).
- **Go 1.24+** (for building the server binary from source).
- **Node.js 22+** (only needed when rebuilding the embedded SvelteKit Admin UI).
- **Optional**: `pg_dump` and `psql` CLI tools on `PATH` for UI-triggered Backups & Restore.
- **Optional**: Redis server (can also be connected later at runtime via `/_/settings/database`).

---

## Environment Configuration (`.env`)

Create a `.env` file in the working directory next to the `microbase` binary:

```env
PORT=3000
DATABASE_URL=postgres://microbase:microbase@localhost:5432/microbase?sslmode=disable
JWT_SECRET=replace-with-a-long-random-secret-at-least-32-chars
REDIS_URL=
```

| Variable | Required | Default | Description |
|---|---|---|---|
| `PORT` | No | `3000` | HTTP listen port. |
| `DATABASE_URL` | Yes | — | PostgreSQL connection URI. Microbase automatically runs schema migrations and enables `pgvector` on startup. |
| `JWT_SECRET` | Yes | — | Signing key for JWT access tokens (generate with `openssl rand -hex 32`). Never leave empty in production. |
| `REDIS_URL` | No | `""` | Optional initial Redis URI (`redis://localhost:6379/0`). Leave empty for in-memory cache; can be hot-swapped live in **Settings → Database & Cache**. |

---

## Option 1: Docker / Docker Compose

```yaml
services:
  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: microbase
      POSTGRES_PASSWORD: change_me
      POSTGRES_DB: microbase
    volumes:
      - pgdata:/var/lib/postgresql/data
    restart: unless-stopped

  microbase:
    image: ghcr.io/techasit5415/microbase:latest
    depends_on:
      - postgres
    ports:
      - "3000:3000"
    environment:
      PORT: "3000"
      DATABASE_URL: "postgres://microbase:change_me@postgres:5432/microbase?sslmode=disable"
      JWT_SECRET: "replace-with-openssl-rand-hex-32"
    volumes:
      - ./mb_data:/app/uploads
    restart: unless-stopped

volumes:
  pgdata:
```

---

## Option 2: Building the Single Binary from Source

```bash
# 1. Build the static SvelteKit Admin UI into ui/build
cd ui
npm install
npm run check
npm run build
cd ..

# 2. Compile a static Go binary with embedded UI (atomic swap avoids "text file busy")
CGO_ENABLED=0 go build -trimpath \
  -ldflags="-s -w -X github.com/techasit5415/microbase/internal/config.Version=v0.10.66" \
  -o microbase.new ./cmd/server && mv -f microbase.new microbase

# 3. Start Microbase
./microbase
```

---

## First-Time Superuser Setup (8-Digit OTP)

When Microbase starts with an empty `_superusers` table, it generates a cryptographically random **8-digit Setup OTP** valid for **5 minutes** and prints it to stdout:

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
4. To add more Superusers later, sign in to `/_/` and open **System → `_superusers` → `+ New record`**.

---

## Production Deployment

### Linux `systemd` Service

Create `/etc/systemd/system/microbase.service`:

```ini
[Unit]
Description=Microbase BaaS
After=network.target postgresql.service

[Service]
Type=simple
User=microbase
WorkingDirectory=/opt/microbase
EnvironmentFile=/opt/microbase/.env
ExecStart=/opt/microbase/microbase
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now microbase
sudo journalctl -u microbase -f
```

### Nginx Reverse Proxy (with SSE Realtime Support)

```nginx
server {
    listen 80;
    server_name api.example.com;

    client_max_body_size 50M;

    # Server-Sent Events (/api/realtime) must disable proxy buffering
    location /api/realtime {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 24h;
    }

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Caddy Reverse Proxy

```caddyfile
api.example.com {
    reverse_proxy 127.0.0.1:3000 {
        flush_interval -1
    }
}
```

---

## Next Steps

- Explore the **[Admin Dashboard Guide](admin-ui.md)** (`/_/collections`, `/_/logs`, `/_/sql`, `/_/settings/:section`).
- Configure **[API Rules & Access Control](api-rules.md)** before exposing collections to client apps.
- Browse the full **[REST API Reference](api-reference.md)**.
