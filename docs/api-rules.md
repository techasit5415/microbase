# API Rules & Access Control

Microbase uses **PocketBase-style API Rules** to control who can access or modify records in every collection. Rules are compiled directly into parameterized PostgreSQL `WHERE` clauses (row-level security) for `list`, `view`, `update`, and `delete`, and evaluated against the incoming payload for `create`.

> **Secure by default:** Every newly created collection starts with all 5 rules **locked** (Superusers only) until you explicitly unlock them.

---

## 1. The 5 Rule Actions

Each collection has 5 independent rules configurable in **Admin UI → Collection Settings → API Rules** or via `PATCH /api/collections/:name`:

| Action | Endpoint | What it controls |
|---|---|---|
| `list` | `GET /api/collections/:name/records` | Who can query/list records (and which rows are returned) |
| `view` | `GET /api/collections/:name/records/:id` | Who can read a single record (also enforced during `?expand=...`) |
| `create` | `POST /api/collections/:name/records` | Who can insert a new record (evaluated against request payload) |
| `update` | `PATCH /api/collections/:name/records/:id` | Who can modify an existing record (and which rows match) |
| `delete` | `DELETE /api/collections/:name/records/:id` | Who can delete an existing record |

> **Superuser bypass:** Authenticated Superusers (`role = "superuser"`) **always** bypass all API rules.

---

## 2. Quick Rule Shorthands

You can set any rule to one of these built-in shorthands or a custom expression:

| Value | Meaning |
|---|---|
| *(Locked / omitted / `"locked"` / `"superuser"`)* | **Superusers only** — all non-superuser requests receive `403 Forbidden` |
| `""` *(empty string)* or `"public"` | **Public** — anyone (including unauthenticated guests) |
| `"auth"` | **Authenticated users** — any valid logged-in JWT |
| `"admin"` | **Admins only** — users with `role = "admin"` or Superusers |
| `<expression>` | **Custom filter expression** — evaluated per request / per row |

---

## 3. Expression Syntax

### Operators

| Operator | Description | Example |
|---|---|---|
| `=` | Equal | `status = "published"` |
| `!=` | Not equal | `@request.auth.id != ""` |
| `>` `>=` `<` `<=` | Numeric / Date comparison | `price >= 100` |
| `~` | Contains (case-insensitive `ILIKE`) | `email ~ "@kmitl.ac.th"` |
| `!~` | Does not contain | `title !~ "draft"` |
| `&&` | Logical AND | `published = true && active = true` |
| `||` | Logical OR | `author = @request.auth.id || @request.auth.role = "admin"` |
| `( ... )` | Grouping | `status = "open" && (owner = @request.auth.id || @request.auth.role = "admin")` |

### Request Macros

| Macro | Value |
|---|---|
| `@request.auth.id` | Authenticated user's ID (works with both `TEXT` and `BIGSERIAL` IDs; `""` if guest) |
| `@request.auth.email` | Authenticated user's email (`""` if guest) |
| `@request.auth.role` | Authenticated user's role (`"user"`, `"admin"`, `"superuser"`, or `""`) |
| `@request.body.<field>` | Value of `<field>` in the incoming JSON body (`create` / `update`) |

---

## 4. Common Recipes (Copy & Paste)

### Public Read, Authenticated Write
- **List / View**: `""` *(empty = public)*
- **Create**: `@request.auth.id != ""`
- **Update / Delete**: `author = @request.auth.id`

### Owner-Only Private Data (e.g. `notes`, `bookings`, `orders`)
- **List / View**: `user_id = @request.auth.id`
- **Create**: `@request.auth.id != "" && @request.body.user_id = @request.auth.id`
- **Update / Delete**: `user_id = @request.auth.id`

### Owner OR Admin Access
- **List / View / Update / Delete**:
  ```text
  user_id = @request.auth.id || @request.auth.role = "admin"
  ```

### Published Items Visible to Everyone, Drafts Only to Author
- **List / View**:
  ```text
  status = "published" || author = @request.auth.id
  ```

### Restrict to a Specific Email Domain
- **Create**:
  ```text
  @request.auth.id != "" && @request.auth.email ~ "@kmitl.ac.th"
  ```

### `users` Collection Self-Access
- **List / View**: `id = @request.auth.id || @request.auth.role = "admin"`
- **Create** *(Public registration)*: `""`
- **Update**: `id = @request.auth.id`
- **Delete**: *(Locked — Superusers only)*

---

## 5. Setting Rules via REST API

```bash
curl -X PATCH http://localhost:3000/api/collections/posts \
  -H "Authorization: Bearer $SUPERUSER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "api_rules": {
      "list": "",
      "view": "",
      "create": "@request.auth.id != \"\"",
      "update": "author = @request.auth.id || @request.auth.role = \"admin\"",
      "delete": "author = @request.auth.id || @request.auth.role = \"admin\""
    }
  }'
```

To **lock** a rule back to Superusers only, omit that key from `api_rules` or set it to `"locked"`.

---

## 6. How Rules Interact with Other Features

- **Relation Expansion (`?expand=...`)**: Expanding a related collection automatically enforces that target collection's **`view`** rule.
- **Hidden Fields**: Fields marked `hidden: true` are automatically stripped from API responses for non-superusers, even when a rule passes.
- **Realtime SSE (`/api/realtime`)**: Subscriptions require passing the collection's `list` / `view` rule.
- **Rate Limiting**: API rules run after the per-route Rate Limiter (which can be toggled live under **Settings → Application → Rate limiting**).


---

## PocketBase Parity: Any-Of Operators, Relation Traversal & Modifiers

Microbase compiles full PocketBase-style expressions into parameterized PostgreSQL SQL (`EXISTS` / `JSONB`):

### Any-Of Operators (Multi-Relation / JSONB Arrays)

| Operator | Description | Example |
|---|---|---|
| `?=` | Any element equals | `owners.id ?= @request.auth.id` |
| `?!=` | No element equals | `tags ?!= "archived"` |
| `?~` | Any element contains (case-insensitive) | `tags ?~ "gpu"` |
| `?!~` | No element contains | `tags ?!~ "draft"` |
| `?>`, `?>=`, `?<`, `?<=` | Any element comparison | `scores ?>= 80` |

### Special Tokens & Modifiers

| Token / Modifier | Description | Example |
|---|---|---|
| `@now` | Current UTC timestamp (`NOW()` in PostgreSQL) | `(start_at = "" \|\| start_at <= @now) && (end_at = "" \|\| end_at > @now)` |
| `@request.auth.collectionName` | Auth collection name (`"users"`, `"_superusers"`, or `""`) | `@request.auth.collectionName = "_superusers"` |
| `@request.auth.<field>` | Direct field on the authenticated `users` record | `@request.auth.user_type ?= "000000000000009"` |
| `@request.auth.<rel>.<field>` | Relation traversal from the authenticated `users` record | `@request.auth.user_type.type ?= "admin"` |
| `<rel>.<field>` / `<rel1>.<rel2>.<field>` | 1- or 2-level relation traversal from the current record | `instance.owners.id ?= @request.auth.id` |
| `@request.body.<field>:isset` | `true` if `<field>` is present in the request JSON body | `@request.body.user_type:isset = false` |
| `@request.body.<field>:changed` | `true` if `<field>` is present and differs from the stored value | `@request.body.status:changed = false` |

> **Empty date strings**: Comparing a `date` or `autodate` column against `""` (`start_at = ""` / `end_at != ""`) is automatically compiled to `IS NULL` / `IS NOT NULL` checks so migrated PocketBase rules never fail on PostgreSQL `TIMESTAMPTZ` columns.
