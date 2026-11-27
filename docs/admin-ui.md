# Admin Dashboard Guide (`/_/`)

Microbase embeds a full PocketBase-inspired Admin UI built with **SvelteKit 3 + Svelte 5 + Tailwind CSS v4** directly inside the Go binary at `/_/`.

---

## Navigation & Routes

Every section in the Admin UI has its own bookmarkable URL:

| Route | Purpose |
|---|---|
| `/_/collections` | Browse collections, inspect/edit records, edit schemas & API rules |
| `/_/collections?collection=users` | Direct link to a specific collection |
| `/_/logs` | Paginated request logs, activity bar chart, Log Details drawer, Logs Settings modal |
| `/_/sql` | Interactive SQL Console + visual ERD Schema Canvas |
| `/_/settings/application` | App metadata, Batch API toggle, Live Rate Limiting, UI Appearance customizer |
| `/_/settings/database` | PostgreSQL connection test/config & Redis cache configuration |
| `/_/settings/mail` | SMTP configuration & test email sender |
| `/_/settings/storage` | S3 / Cloudflare R2 / MinIO object storage settings |
| `/_/settings/backups` | Create, upload, authenticated download, restore, and delete `pg_dump` backups |
| `/_/settings/crons` | Manage scheduled background jobs and run them on demand |
| `/_/settings/webhooks` | Configure Generic HTTP, Discord, and Firebase FCM webhooks |
| `/_/settings/export` | Export all collections, fields, indexes, and API rules to JSON |
| `/_/settings/import` | Import Microbase JSON or PocketBase `pb_schema.json` (paste or file upload) |

Visiting `/_/settings` automatically redirects to `/_/settings/application`.

---

## Collections & Records (`/_/collections`)

### Resizable & Pinnable Sidebar
- Drag the vertical divider on the right edge of the Collections sidebar (`180px`–`480px`). Width is saved automatically in `localStorage`.
- **Pin Collections (📌)**: Hover any collection and click the **Pin** icon to pin frequently used tables to a dedicated **Pinned** section at the top of the sidebar (persisted in `localStorage`).
- Collections are grouped into **Pinned**, **Collections** (user-created tables/views/auth), and **System** (`_superusers`, `_authOrigins`, `_oauth_accounts`, `_oauth_providers`, `_webhooks`).

### Records Table
- **Copyable ID Pills**: Click any record ID pill to copy it to the clipboard.
- **Column Sorting**: Click any column header to toggle `ASC` / `DESC` sorting without table flicker.
- **Relation Pills & Eye Preview (`👁`)**: Expanded single and multi-relations display human-readable `Presentable` labels. Hover the **Eye** icon to preview the related record JSON (auto-flips above when near the bottom of the screen), or **click** the Eye icon to pin and scroll the preview popover.
- **Records Picker Modal**: When creating or editing a record with a `relation` field, click the picker to search, filter, and select one or multiple related records.
- **Pagination Bar**: Jump to first/last/specific pages and change rows per page (`10`, `20`, `30`, `50`, `100`).
- **Impersonate**: On auth collections (such as `users`), open a record or use the Impersonate action to mint a temporary JWT for debugging user-scoped API Rules.

### Collection Schema Editor
- Click any field row to expand its options; use the `⋯` menu on the row to **Duplicate** or **Remove** the field.
- Mark any field as **Presentable** so related tables show that field instead of raw IDs.
- Configure `List`, `View`, `Create`, `Update`, and `Delete` API Rules directly in the **API Rules** tab.

### Collections Overview Modal
- Click the **Collections overview** grid icon next to the sidebar search bar to inspect all collections at a glance (toggle **System collections** to include internal tables):
  - **Fields and relations**: Interactive ERD Canvas showing fields and foreign-key relation lines across collections.
  - **Rules**: Cross-collection matrix for `List/Search`, `View`, `Create`, `Update`, and `Delete` rules with `Public` / `Superusers only` badges, syntax-highlighted expressions, and one-click jump directly into that collection's **API Rules** editor.

---

## Logs (`/_/logs`)

- **Level Badges**: Each request is tagged with `• INFO (0)` (2xx/3xx), `• WARN (4)` (4xx), `• ERROR (8)` (5xx), or `• DEBUG (-4)`.
- **Auth Identity**: Shows which auth collection (`_superusers`, `users`, or custom auth collection) and email/ID made the request. Unauthenticated requests display `—` (`N/A` in details).
- **Server-Side Search & Pagination**: Type in the search bar (250ms debounce) to search across all persisted logs in PostgreSQL by level (`INFO`, `WARN`, `ERROR`), method, URL, auth collection, email, IP, or status code.
- **Log Details Drawer**: Click any row to open the slide-over **Log details** drawer with full metadata (`execTime`, `auth`, `authEmail`, `authId`, `referer`, `remoteIP`, `userIP`, `userAgent`) and a **Download JSON** button.
- **Logs Settings Modal** (gear icon next to Refresh):
  - **Max days retention**: `0` disables log persistence and clears all logs; `1–365` sets retention days.
  - **Min log level**: Clickable presets (`-4:DEBUG`, `0:INFO`, `4:WARN`, `8:ERROR`).
  - **Max data size (bytes)**: Per-field cap (defaults to `~16KB`).
  - **Enable IP logging** & **Enable Auth Id logging** switches.
  - **Trash icon**: Clear all persisted logs (with confirmation dialog).

---

## SQL Console & ERD Canvas (`/_/sql`)

- **SQL Console**: Run ad-hoc PostgreSQL queries directly against your database with tabular results and execution time.
- **Interactive ERD Canvas**: Drag tables to arrange your schema, pan/zoom the canvas, and inspect live SVG Bezier lines connecting foreign-key `relation` fields to their target tables.

---

## UI Appearance Customizer (`/_/settings/application`)

All appearance preferences are stored locally in the browser (`localStorage: microbase_ui_prefs`) and update live with a built-in mini preview:

- **Theme**: `Light`, `Dark`, or `System` (synced with the sidebar theme toggle)
- **Font Family**: `IBM Plex Sans Thai` (default), `Inter`, or `System`
- **Font Sizes**: Independent sliders for **Base font size** (`14–20px`), **Sidebar font size** (`12–19px`), and **Table data font size** (`12–20px`)
- **Table Row Density**: `Compact`, `Normal`, `Comfortable`
- **Corner Radius**: `Sharp`, `Default`, `Rounded`
- **Date Format**: `Local`, `UTC`, or `Relative`
- **Default Rows Per Page**: `10`, `20`, `30`, `50`, or `100`


### API Preview Drawer
- Click **API Preview** on any collection to view ready-to-copy snippets for **JS SDK** (`@techasit5415/microbase-sdk`, with automatic `Authorization: Bearer` from `mb.authStore.token`), **JS (fetch)**, **TypeScript**, **curl**, and **Go**, plus full `?query params` documentation (`page`, `perPage`, `sort` including `@random`, `filter`, `search`, `expand`, `fields`, `skipTotal`).
