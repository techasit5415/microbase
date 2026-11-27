# Authentication, OAuth2/OIDC & API Rules

## Superusers (`_superusers`)

Superusers have full administrative access and automatically bypass all collection API rules. Superuser login is **email/password only** (end-user OAuth2 buttons are intentionally excluded from `/_/`).

| Endpoint | Auth | Description |
|---|---|---|
| `GET /api/superusers/status` | Public | Returns `{ "has_superuser": true/false }`. |
| `POST /api/superusers/setup` | Setup OTP | Creates the first superuser (requires 8-digit 5m OTP; invalidated after 5 failed attempts, rate-limited 5 req/min). Permanently locked once created. |
| `POST /api/superusers/login` | Public | Returns a Superuser Bearer JWT (`role: "superuser"`). |
| `POST /api/superusers` | Superuser | Creates an additional superuser account. |
| `PATCH /api/superusers/:id` | Superuser | Updates a superuser's email or password. |
| `DELETE /api/superusers/:id` | Superuser | Deletes a superuser (prevents deleting the last remaining superuser). |

## End-User Authentication (`users`)

| Endpoint | Auth | Description |
|---|---|---|
| `GET /api/auth/methods` | Public | Lists password login & enabled OAuth2/OIDC providers. |
| `POST /api/auth/register` | Public | Registers a new user (`email`, `password` min 8 chars). |
| `POST /api/auth/login` | Public | Authenticates and returns `{ token, user }`. |
| `GET /api/auth/me` | Bearer JWT | Returns the currently authenticated user. |
| `POST /api/auth/refresh` | Bearer JWT | Issues a fresh JWT. |
| `POST /api/auth/request-verification` | Public | Sends a 15-minute email verification token. |
| `POST /api/auth/confirm-verification` | Public | Confirms token and sets `verified = true`. |
| `POST /api/auth/request-password-reset` | Public | Sends a 15-minute password reset token. |
| `POST /api/auth/confirm-password-reset` | Public | Resets password using `{ token, password }`. |
| `POST /api/auth/request-email-change` | Bearer JWT | Sends a 15-minute token to `new_email`. |
| `POST /api/auth/confirm-email-change` | Public | Confirms email change using `{ token }`. |
| `POST /api/auth/request-otp` | Public | Sends a 6-digit login OTP (valid 5m, rate-limited 10 req/min) and returns `otp_id`. |
| `POST /api/auth/auth-with-otp` | Public | Exchanges `{ otp_id, password }` for a JWT (invalidated after 5 failed attempts, rate-limited 10 req/min). |
| `POST /api/users/:id/impersonate` | Superuser | Mints a Bearer JWT impersonating user `:id` for API testing. |

*(When SMTP Mail is disabled in Settings, verification/reset/OTP tokens are printed to the server log for easy local development. Bearer JWTs must be sent via the `Authorization: Bearer <token>` header; `?token=` query parameter is only accepted on `/api/realtime` SSE. On `users` and custom `auth` collections, `email` is hidden from non-owners/non-superusers unless `email_visibility = true`; non-privileged users cannot set `role` or `verified`, and changing `email` via `PATCH /api/users/:id` automatically resets `verified = false`.)*

### Login Fingerprints (`_authOrigins`)

Each successful superuser or user login records a device/origin fingerprint (`SHA-256(IP|User-Agent)`) in the `_authOrigins` system collection (matching PocketBase's `_authOrigins`), viewable and manageable under **System -> `_authOrigins`**.

---

## OAuth2 & OIDC (Google, Discord, Standard OIDC & Custom IAM)

Configure providers in `/_/` -> **Collections -> `users` -> ⚙️ -> Options -> OAuth2**.

### External App Login Flow (One-Time Code Exchange)

1. Redirect your frontend to:
   ```text
   GET /api/auth/oauth/<provider>?redirect_to=https://your-app.com/auth/callback
   ```
   *(For security, `redirect_to` must match the origin of `AppURL` in Application Settings or an origin listed in `CORS_ORIGINS`.)*
2. After the user authenticates with the IdP, Microbase redirects back to:
   ```text
   https://your-app.com/auth/callback?code=<ONE_TIME_CODE>
   ```
3. Exchange the 60-second one-time code for a JWT:
   ```bash
   curl -X POST "http://localhost:3000/api/auth/oauth/exchange" \
     -H "Content-Type: application/json" \
     -d '{"code":"<ONE_TIME_CODE>"}'
   ```

### Standard OIDC (`oidc`)

For standard OIDC providers (Keycloak, Authentik, Auth0, Okta, Google):
- Fill **Client ID**, **Client Secret**, and **OIDC Issuer URL**.
- Leave **Auth URL**, **Token URL**, and **UserInfo URL** blank for automatic discovery via `<issuer_url>/.well-known/openid-configuration`, or set them explicitly if needed.
- **Strict Verification**: When OIDC discovery is enabled, `id_token` signature, audience, issuer, and expiry verification is mandatory (fail-closed — invalid tokens are rejected immediately). For Google and Discord providers, the provider must report a verified email (`email_verified: true` / `verified: true`) before account linking is permitted.

### KMITL Science IAM (`scikmitl`) & Custom University IAM

Microbase includes a dedicated **`scikmitl`** provider pre-configured with KMITL Science IAM v2 endpoints:
- **Auth URL**: `https://iam.science.kmitl.ac.th/o/oauth2/auth`
- **Token URL**: `https://api.science.kmitl.ac.th/iam/oidc2/token`
- **UserInfo URL**: `https://api.science.kmitl.ac.th/iam/oidc2/userinfo`
- **Scopes**: `openid profile email`

#### Optional Profile Enrichment API
If your IAM exposes extra user attributes on a separate endpoint, you can configure **Profile Enrichment API URL** and **API Key** (sent as `Authorization: Bearer <key>`):
- Supports URL template placeholders: `{user_id}`, `{sub}`, `{email}`, `{student_id}`, or any dot-path claim (e.g. `{data.id}`).
- Automatically unwraps `{ "data": { ... } }` envelopes while keeping `data.*` intact.
- Non-2xx responses (e.g. `403 Forbidden`) are safely ignored so error payloads never pollute user claims.

---

### JSON Path Syntax (Claim Mapping & Access Restrictions)

Both **Claim / Field Mapping** and **Access Restriction** share the same JSON path resolver:

| Syntax | Example | Behavior |
|---|---|---|
| Dot-path | `profile.departmentId` | Traverses nested JSON objects. |
| Array index | `student_profile.0.department` | Selects a specific 0-based index from a JSON array. |
| Implicit array traversal | `profile.profiles.department.id` | Automatically walks every element in a JSON array (`profiles[]`) and collects matching values. |
| Explicit wildcard | `profile.profiles.*.department.id` | Same as implicit array traversal; matches any element in the array. |
| Fallback paths (`\|`) | `profile.departmentId \| profile.profiles.department.id` | Evaluates left-to-right. In **Field Mapping**, uses the first non-empty value. In **Access Restrictions**, passes if **any** path/element matches. |

---

### Claim / Field Mapping (`users column <- IAM JSON path`)

Maps claims from `id_token`, `userinfo`, or the Profile Enrichment API into columns on the `users` collection on every OAuth login.

Example mappings:
- `name` <- `name | profile.firstNameTh | profile.firstname_th`
- `iam_role` <- `role`
- `department` <- `profile.departmentId | profile.profiles.department.id`

> **Protected Columns**: For security, OIDC Claim Mapping **never overwrites** `id`, `email`, `password_hash`, or `role` on `users`. This prevents external OIDC claims (such as `role: "admin"`) from escalating privileges.
>
> **Recommended pattern for IAM roles (`student` / `staff` / `guest`)**:
> 1. Add a custom `text` field named `iam_role` to the `users` collection.
> 2. Add a Claim Mapping: `iam_role` <- `role`.
> 3. In your app's OAuth callback, read `user.iam_role` when assigning a default application role (e.g. `user_type`) for new users, without overwriting existing manually assigned roles like `admin`.

---

### Access Restrictions (`Allowed IAM Claims`)

Restricts who is allowed to sign in via OAuth2/OIDC. Users who fail any rule are rejected with `403` (`OAUTH_FAILED`) **before** a `users` record is created.

- **Multiple rows = `AND`**: Every configured rule row must pass.
- **Comma-separated values in one row = `OR`**: Any listed value (case-insensitive) satisfies that row.
- **Do not duplicate the same claim key on two rows**: Rules are stored as a JSON map (`{ "claim_path": ["val1", "val2"] }`), so a second row with the same key overwrites the first. Always combine multiple allowed values for the same key into **one comma-separated row**.

#### Example: Restrict to One Department (Students & Staff)

For example, if your IAM `/oidc2/userinfo` returns:
- Students have `role: "student"` and `profile.departmentId: "dept-cs-01"`.
- Staff/lecturers have `role: "staff"` and `profile.profiles[].department.id: "dept-cs-01"`.

Configure two rows in **Access Restriction**:

| Claim Path | Allowed Values (comma-separated) |
|---|---|
| `role` | `student, staff` |
| `profile.departmentId \| profile.profiles.department.id` | `dept-cs-01` |

- CS students (`role = "student"`, `profile.departmentId = "dept-cs-01"`) -> **Allowed**
- CS lecturers/staff (`role = "staff"`, any entry in `profile.profiles[]` with `department.id = "dept-cs-01"`) -> **Allowed**
- Other departments or `guest` accounts -> **Blocked** (`OAUTH_FAILED`)

---

## API Rules (Secure by Default)

Microbase follows PocketBase's **secure-by-default** model:

- **Locked (default / key omitted)**: **Superusers only** (`403 Forbidden` for all non-superusers).
- **Unlocked + empty string (`""`)**: **Public** (anyone can access).
- **`auth`** or **`@request.auth.id != ''`**: Any authenticated user.
- **`admin`**: Users with `role = 'admin'` or `_superusers`.

Configure rules per action (`list`, `view`, `create`, `update`, `delete`) in the Collection Drawer's **API rules** tab.
