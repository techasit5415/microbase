# Authentication, OAuth2/OIDC & API Rules

## Superusers (`_superusers`)

Superusers have full administrative access and automatically bypass all collection API rules.

| Endpoint | Auth | Description |
|---|---|---|
| `GET /api/superusers/status` | Public | Returns `{ "has_superuser": true/false }`. |
| `POST /api/superusers/setup` | Setup OTP | Creates the first superuser (requires 8-digit 5m OTP). Permanently locked once created. |
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
| `POST /api/auth/request-otp` | Public | Sends a 6-digit login OTP (valid 5m) and returns `otp_id`. |
| `POST /api/auth/auth-with-otp` | Public | Exchanges `{ otp_id, password }` for a JWT. |
| `POST /api/users/:id/impersonate` | Superuser | Mints a Bearer JWT impersonating user `:id` for API testing. |

*(When SMTP Mail is disabled in Settings, verification/reset/OTP tokens are printed to the server log for easy local development.)*

## OAuth2 & OIDC (Google, Discord, Standard OIDC & Custom IAM)

Configure providers in `/_/` -> **Collections -> `users` -> ⚙️ -> Options -> OAuth2**.

### External App Login Flow (One-Time Code Exchange)

1. Redirect your frontend to:
   ```text
   GET /api/auth/oauth/<provider>?redirect_to=https://your-app.com/auth/callback
   ```
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

### Standard OIDC (Auto-Discovery)

For standard OIDC providers (Keycloak, Authentik, Auth0, Okta, Google):
- Fill **Client ID**, **Client Secret**, and **OIDC Issuer URL**.
- Leave **Auth URL**, **Token URL**, **UserInfo URL**, and **Claim / Field Mapping** blank.
- Microbase automatically discovers endpoints from `<issuer_url>/.well-known/openid-configuration` and verifies `id_token`.

### Custom IAM / Non-Standard OAuth2 (e.g. University IAM)

For custom IAM systems without `.well-known/openid-configuration`:
1. Set **Display name** (e.g. `KMITL IAM`) and **Scopes**.
2. Fill **Auth URL**, **Token URL**, and **UserInfo / Profile API URL** (e.g. `https://api.science.kmitl.ac.th/iam/o/v2/students`).
3. Add **Claim / Field Mapping** rules (`_users column <- IAM JSON dot-path`):
   - `sub <- student_id`
   - `email <- email`
   - `name <- firstname_th`
   - `department <- department.name`
   - `major <- major.name`
4. On login, Microbase fetches the UserInfo JSON with the access token and automatically syncs mapped values into `_users`.

## API Rules (Secure by Default)

Microbase follows PocketBase's **secure-by-default** model:

- **Locked (default / key omitted)**: **Superusers only** (`403 Forbidden` for all non-superusers).
- **Unlocked + empty string (`""`)**: **Public** (anyone can access).
- **`auth`** or **`@request.auth.id != ''`**: Any authenticated user.
- **`admin`**: Users with `role = 'admin'` or `_superusers`.

Configure rules per action (`list`, `view`, `create`, `update`, `delete`) in the Collection Drawer's **API rules** tab.
