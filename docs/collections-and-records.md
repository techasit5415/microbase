# Collections, Fields, Querying & Realtime

## Collection Types

When creating a collection (`POST /api/collections` or **+ New collection** in `/_/`), choose from three types:

| Type | PostgreSQL Object | Capabilities |
|---|---|---|
| `base` | `CREATE TABLE` | Full CRUD, custom fields, indexes, all 5 API rules. |
| `view` | `CREATE OR REPLACE VIEW` | Read-only SQL `SELECT` / `WITH` query, `List/Search` and `View` API rules. |
| `auth` | `CREATE TABLE` with auth columns | Built-in `email`, `password_hash`, `verified`, `email_visibility`, `role` + custom fields, OAuth2/OTP/MFA options. |

The built-in `users` collection maps to `_users` and is fully editable — you can add/remove custom columns (e.g. `name`, `avatar`, `department`), configure API rules, and attach unique indexes.

## Primary Key Modes & Autogenerate Patterns

Each new collection supports two Primary Key modes (click the gear icon on the `id` row):

1. **Text (PocketBase style, default)**: `"id" TEXT PRIMARY KEY` with an `id_autogenerate_pattern` (default `[a-z0-9]{15}`). Supports regex-like patterns such as `ord_[0-9]{8}`, `[A-Z0-9]{10}`, or `uuid` (UUIDv4).
2. **Auto-Increment**: `"id" BIGSERIAL PRIMARY KEY`.

Any `text` field also supports `autogenerate_pattern`. If a record is created without a value for that field, Microbase generates one automatically before validation.

## Supported Field Types

| Field Type | PostgreSQL Type | Key Options |
|---|---|---|
| `text` | `TEXT` | `min`, `max`, `pattern`, `autogenerate_pattern` |
| `editor` | `TEXT` | `max_size`, WYSIWYG formatting toolbar |
| `number` / `float` | `DOUBLE PRECISION` (or `BIGINT` if `only_int`) | `min`, `max`, `only_int` |
| `integer` | `BIGINT` | `min`, `max` |
| `boolean` | `BOOLEAN` | Switch control |
| `email` | `TEXT` | `only_domains`, `except_domains` |
| `url` | `TEXT` | `only_domains`, `except_domains` |
| `datetime` / `timestamptz` | `TIMESTAMPTZ` | Date/time values |
| `autodate` | `TIMESTAMPTZ DEFAULT NOW()` | `on_create`, `on_update` |
| `file` | `TEXT` / `JSONB` (Multiple) | `max_select`, `mime_types`, `thumbs`, `max_size`, `protected` |
| `relation` | `BIGINT` (`REFERENCES`) / `JSONB` (Multiple) | `collection_name`, `max_select`, `cascade_delete` |
| `select` | `TEXT` / `JSONB` (Multiple) | `values` (required choices), `max_select` |
| `json` | `JSONB` | `max_size` |
| `geopoint` | `JSONB` (`{"lat": 13.75, "lon": 100.50}`) | Interactive OpenStreetMap preview |
| `vector` | `vector(N)` (`pgvector`) | `dimension` (`1..4096`) |

Every field also supports `required` (`NOT NULL`), `presentable`, `hidden` (stripped from non-superuser API responses), `unique`, and `help_text`.

## Records CRUD API

Base path: `/api/collections/:name/records`

- `GET /api/collections/:name/records` — List / search / filter records.
- `GET /api/collections/:name/records/:id` — Get single record.
- `POST /api/collections/:name/records` — Create record.
- `PATCH /api/collections/:name/records/:id` — Update record.
- `DELETE /api/collections/:name/records/:id` — Delete record.
- `POST /api/collections/:name/truncate` — Delete all records in the table.
- `POST /api/collections/:name/duplicate` — Clone collection schema.

### Query Parameters

| Param | Example | Description |
|---|---|---|
| `page` | `page=1` | Page number (default `1`). |
| `per_page` | `per_page=30` | Items per page (default `20`, max `100`). |
| `sort` | `sort=-created_at,id` | Sort columns; prefix `-` for DESC. |
| `search` | `search=สวัสดี` | English Full-Text Search + Thai `ILIKE` across all text columns. |
| `filter` | `filter=(views>10 && published=true)` | PocketBase filter expression (see below). |
| `expand` | `expand=author,category` | Resolves relation fields into `record.expand.<field>`. |
| `fields` | `fields=id,title,created_at` | Returns only the specified fields. |

### Filter Expression Syntax (`?filter=...`)

Supported operators:
- Comparison: `=`, `!=`, `>`, `>=`, `<`, `<=`
- Pattern matching: `~` (`ILIKE '%val%'`), `!~` (`NOT ILIKE '%val%'`)
- Logical: `&&` (`AND`), `||` (`OR`), grouping with `(...)`
- Literals: `'string'`, `"string"`, numbers, `true`, `false`, `null`
- Request variables: `@request.auth.id`, `@request.auth.email`, `@request.auth.role`

Example:
```bash
curl -G "http://localhost:3000/api/collections/posts/records" \
  --data-urlencode "filter=(published=true && views>=50) || title~'microbase'" \
  --data-urlencode "expand=author" \
  --data-urlencode "fields=id,title,views,expand"
```

## Vector Similarity Search (`pgvector`)

```bash
curl -X POST "http://localhost:3000/api/collections/documents/vector-search" \
  -H "Content-Type: application/json" \
  -d '{
    "field": "embedding",
    "vector": [0.12, -0.34, 0.56],
    "limit": 10
  }'
```

## Realtime Subscriptions (SSE)

```javascript
const es = new EventSource('http://localhost:3000/api/realtime?collection=posts');
es.onmessage = (event) => {
  const payload = JSON.parse(event.data);
  console.log(payload.action, payload.collection, payload.record);
};
```

## Batch API

Execute up to 50 sub-requests in one HTTP call via `POST /api/batch`:

```bash
curl -X POST "http://localhost:3000/api/batch" \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "requests": [
      { "method": "POST", "url": "/api/collections/posts/records", "body": { "title": "First" } },
      { "method": "POST", "url": "/api/collections/posts/records", "body": { "title": "Second" } }
    ]
  }'
```
