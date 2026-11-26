# CLI Flags, Migrations (`mb_migrations`) & JS Hooks (`mb_hooks`)

## 1. CLI Subcommands & Flags

```bash
microbase [command] [flags]
```

### Commands
- `microbase serve` *(default)* — Start the Microbase HTTP server.
- `microbase update [--check] [--to <tag>]` *(or `--update`)* — Check or self-update the binary from GitHub Releases with SHA-256 verification and atomic swap.
- `microbase superuser upsert <email> <password>` — Create or reset a superuser account directly from the CLI.
- `microbase migrate <up|down|status|create <name>|snapshot>` — File-based PostgreSQL & schema migrations.
- `microbase version` *(or `-v` / `--version`)* — Print current version and exit.

### Serve / Global Flags
| Flag | Env Var | Default | Description |
|---|---|---|---|
| `-v, --version` | — | — | Print version and exit |
| `-e, --env <path>` | — | `.env` | Load a specific `.env` file |
| `--db <url>` | `DATABASE_URL` | — | Override PostgreSQL connection URL |
| `--dir <path>` | `DATA_DIR` | `./storage` | Local storage directory |
| `--http <addr>` | — | `0.0.0.0:3000` | Listen address (`host:port`) |
| `--host <host>` | `HOST` | `0.0.0.0` | Listen host |
| `--port <port>` | `PORT` | `3000` | Listen port |
| `--origins <list>` | `CORS_ORIGINS` | `*` | Comma-separated CORS allowed origins |
| `--no-ui` | `DISABLE_ADMIN_UI=true` | `false` | Disable embedded Admin UI (`/_/`) for headless API-only deployments |
| `--dev` | `DEV=true` | `false` | Enable verbose debug logging |
| `--hooksDir <path>` | `HOOKS_DIR` | `./mb_hooks` | Directory for embedded JS hooks |
| `--migrationsDir <path>` | `MIGRATIONS_DIR` | `./mb_migrations` | Directory for `.sql` / `.json` migrations |

---

## 2. File-Based Migrations (`mb_migrations/`)

Applied migrations are tracked in PostgreSQL table `_migrations`.

```bash
# Scaffold a new SQL migration with Up/Down sections
./microbase migrate create add_posts_index

# Export current _collections schema into a versioned JSON snapshot
./microbase migrate snapshot

# Apply all pending .sql and .json migrations
./microbase migrate up

# Roll back the most recent .sql migration (-- +migrate Down)
./microbase migrate down

# Inspect applied & pending status
./microbase migrate status
```

---

## 3. Embedded JavaScript Hooks (`mb_hooks/*.js`)

Place `.js` files in `./mb_hooks` (or `--hooksDir`). Microbase executes them in a sandboxed Goja JS runtime (5-second execution timeout per invocation).

### Example: `mb_hooks/main.js`

```javascript
// Mutate or reject records before creation
onRecordBeforeCreate("posts", (e) => {
  if (!e.data.title) {
    throw new Error("title is required");
  }
  e.data.slug = e.data.title.toLowerCase().replace(/\s+/g, "-");
});

// React after creation
onRecordAfterCreate("posts", (e) => {
  console.log("Created post:", e.record.id);
});

// Mount a custom HTTP endpoint on Microbase
routerAdd("GET", "/api/custom/stats", (c) => {
  const rows = $db.query("SELECT COUNT(*) AS total FROM users");
  return { users: rows[0]?.total ?? 0, requestedBy: c.query("who") || "anon" };
});
```

### Available Globals
- **Lifecycle Hooks**: `onRecordBeforeCreate`, `onRecordAfterCreate`, `onRecordBeforeUpdate`, `onRecordAfterUpdate`, `onRecordBeforeDelete`, `onRecordAfterDelete` (pass `"*"` or omit collection name to match all collections).
- **Custom Routes**: `routerAdd(method, path, (c) => ...)` with `c.param()`, `c.query()`, `c.header()`, `c.body`, `c.status(code)`.
- **Database**: `$db.query(sql, ...args)` and `$db.exec(sql, ...args)`.
- **Outbound HTTP**: `$http.send({ url, method, headers, body })`.
- **Logging**: `console.log()`, `console.warn()`, `console.error()`.

---

## 4. Thumbnails, Protected File Tokens & Advanced Query Modifiers

- **On-the-fly Image Thumbnails** (`GET /api/files/:filename?thumb=...`, bilinear resampling + cached):
  - `100x100` (center crop), `100x100t` (top crop), `100x100b` (bottom crop), `100x100f` (fit without crop), `0x200` / `200x0` (proportional).
- **Protected File Tokens**:
  - `POST /api/files/token` (requires auth) → returns a 2-minute signed `{ "token": "..." }`.
  - Pass `?token=<token>` on `GET /api/files/:filename?token=...`.
- **Field Modifiers (`?fields=`)**:
  - `*` wildcard, nested `expand.<rel>.<field>` projection, and `:excerpt(maxLen, withEllipsis)` (e.g. `?fields=*,body:excerpt(200,true)`).
- **Filter Modifiers**:
  - `:lower` (`email:lower = 'user@example.com'`) and `:length` (`tags:length > 0`).
- **Back-Relations & Nested Expand (`?expand=`)**:
  - Multi-level dot paths up to 6 levels (`?expand=instance.email`) and reverse back-relations (`?expand=instances_via_email`).
