# Research: How an API client authenticates against the Ostfalia Taiga

Ticket: [#2](https://github.com/zeckitoz/taiga-tui/issues/2) (part of map [#1](https://github.com/zeckitoz/taiga-tui/issues/1)).
Follow-up HITL check: [#8](https://github.com/zeckitoz/taiga-tui/issues/8).
Researched on 2026-11-11.

Source code citations are pinned to taiga-back commit [`8ab6a4e`](https://github.com/taigaio/taiga-back/tree/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa) (current `main`, 2026-11-03).
Instance facts come from unauthenticated GET requests to `https://taiga-fki.ostfalia.de`.
No login was attempted and no credentials were sent.

## Answer in short

- The Ostfalia web UI logs in with `POST /api/v1/auth` and `{"type": "normal", "username": ..., "password": ...}`.
  The TUI should send exactly the same request.
- `username` accepts either the Taiga username or the e-mail address.
- The response is the user object plus `auth_token` (JWT access token) and `refresh` (JWT refresh token).
  API calls then send `Authorization: Bearer <auth_token>`.
- With upstream defaults the access token lives 24 hours and the refresh token lives 8 days.
  Refresh rotation is on: each `POST /api/v1/auth/refresh` returns a new `auth_token` and a new `refresh`, and the old refresh token is denylisted.
  These values can be overridden per instance, so ticket #8 checks the real `exp` claims.
- Application Tokens exist (`Authorization: Application <token>`, no expiry), but only an instance admin can create the Application in Django admin.
  This makes them unusable for the MVP without help from the Ostfalia admins.
- The instance runs taiga-front 6.10.3 (byte-identical `app.js`).
  The backend version is not exposed publicly, but every API feature checked fits taiga-back 6.10.x.
- Recommended flow: password login once, store only the refresh token in the OS keychain, keep the access token in memory, refresh on 401 or shortly before `exp`, and log in again with a password prompt when the refresh token has expired.

## 1. Login types of `POST /api/v1/auth`

The login view dispatches on `type` ([`taiga/auth/api.py` L67-87](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/auth/api.py#L67-L87)):

- `"normal"` is built in and validates `username` and `password` with `TokenObtainPairSerializer`.
- Any other value must be registered by a backend plugin through `register_auth_plugin(name, login_func)` ([`taiga/auth/services.py` L39-46](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/auth/services.py#L39-L46)).
  Otherwise the API answers `400 invalid login type`.
- Known plugin types are `github` and `gitlab` (official contrib plugins, enabled in the docker image only with public registration plus `ENABLE_GITHUB_AUTH` / `ENABLE_GITLAB_AUTH`, see [`docker/config.py`](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/docker/config.py)), and `ldap` from the third-party [taiga-contrib-ldap-auth-ext](https://github.com/Monogramm/taiga-contrib-ldap-auth-ext) plugin ([`apps.py`](https://github.com/Monogramm/taiga-contrib-ldap-auth-ext/blob/master/taiga_contrib_ldap_auth_ext/apps.py) calls `register_auth_plugin("ldap", ...)`).
- The official API docs describe the same request: [docs.taiga.io/api.html#auth-normal-login](https://docs.taiga.io/api.html#auth-normal-login).

`"normal"` login looks up the user by username **or** e-mail (case-insensitive) and then calls Django `user.check_password(password)`.
It rejects inactive and system users ([`taiga/users/services.py` L36-65](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/users/services.py#L36-L65)).
On failure the API answers `401` with `"No active account found with the given credentials"` ([`taiga/auth/services.py` L68-78](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/auth/services.py#L68-L78)).
An optional `invitation_token` in the same body accepts a project invitation as part of the login.

### Which type does Ostfalia use?

The frontend decides the type, not the user.
In the served `app.js` both login forms call `$auth.login(data, config.get("loginFormType", "normal"))`, and the auth service sets `data.type = type || "normal"`.
The LDAP plugin requires the operator to add `"loginFormType": "ldap"` to `conf.json` ([plugin README](https://github.com/Monogramm/taiga-contrib-ldap-auth-ext/blob/master/README.md)).
The Ostfalia [`conf.json`](https://taiga-fki.ostfalia.de/conf.json) has no `loginFormType` key and `contribPlugins: []`.

**Conclusion:** The Ostfalia web UI sends `type: "normal"`.
Because the TUI uses the same endpoint with the same body, any credentials that work in the web UI also work for the TUI.
We do not need to know how the "Uni-Account" is set up behind the scenes.

**Open point for #8:** The upstream `normal` path only checks the Django password hash.
It does not consult LDAP.
So "Uni-Account" probably means one of these: a Taiga account registered with the university e-mail (`publicRegisterEnabled: true`), accounts provisioned by the admins, or a locally patched backend.
The HITL check in #8 confirms that the login works.

## 2. Token lifetime and refresh

Taiga's auth module has been based on djangorestframework-simplejwt since 6.3.0 ([CHANGELOG](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/CHANGELOG.md)).

Settings that are in effect unless the instance overrides `SIMPLE_JWT`:

| Setting | Value | Source |
|---|---|---|
| `ACCESS_TOKEN_LIFETIME` | 24 hours | [`settings/common.py` L450-454](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/settings/common.py#L450-L454) |
| `REFRESH_TOKEN_LIFETIME` | 8 days | same |
| `ROTATE_REFRESH_TOKENS` | `True` | module default, [`taiga/auth/settings.py` L263-287](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/auth/settings.py#L263-L287) |
| `DENYLIST_AFTER_ROTATION` | `True` | same |
| `AUTH_HEADER_TYPES` | `Bearer` | same |
| Algorithm | HS256, claims `token_type`, `exp`, `jti`, `user_id` | same |

The docker image's [`docker/config.py`](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/docker/config.py) does not override `SIMPLE_JWT`.
The docstring in `taiga/auth/settings.py` shows different numbers (5 min / 1 day), but those are simplejwt's documentation examples and are not used.

The refresh flow ([`taiga/auth/api.py` L88-94](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/auth/api.py#L88-L94), [`taiga/auth/services.py` L81-104](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/auth/services.py#L81-L104)):

```http
POST /api/v1/auth/refresh
Content-Type: application/json

{"refresh": "<refresh token>"}
```

- The response is `{"auth_token": "...", "refresh": "..."}`.
  The new `refresh` has a fresh `exp`, so the refresh window slides: an active client stays logged in indefinitely.
- The old refresh token is denylisted.
  The client must store the new one atomically, or a crash can leave it with a dead token.
- An invalid or expired refresh token gives `401` with code `token_not_valid` ([`taiga/auth/exceptions.py`](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/auth/exceptions.py)).
- The web UI does the same: an HTTP interceptor catches 401, calls `/auth/refresh`, stores `auth_token` and `refresh`, and retries the request (visible in the served `app.js`).
- `POST /api/v1/auth/verify` exists but returns 403 unless `DEBUG` is on.
  Do not rely on it.

Login failures can be throttled (`LoginFailRateThrottle`, scope `login-fail`), but the default rate is `None`, meaning unlimited.
The instance may configure a rate.

## 3. Application Tokens as an alternative

- The backend accepts `Authorization: Application <token>` ([`taiga/external_apps/auth_backends.py`](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/external_apps/auth_backends.py)), and it is listed in `DEFAULT_AUTHENTICATION_CLASSES` ([`settings/common.py` L473-484](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/settings/common.py#L473-L484)).
- Such a token is a `signing.dumps({"app_token_id": ...})` value with no expiry ([`taiga/external_apps/models.py` L81-85](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/external_apps/models.py#L81-L85)).
- To get one, an **Application** must already exist.
  The `/applications` API is read-only (create and update raise `NotSupported`), so Applications can only be created in Django admin ([`taiga/external_apps/admin.py`](https://github.com/taigaio/taiga-back/blob/8ab6a4e3b5f6fbbfcbf108a6449011f07b7308aa/taiga/external_apps/admin.py)).
  After that, the user authorizes the app with a JWT (`POST /application-tokens/authorize`, then `POST /application-tokens/validate`), see [docs.taiga.io/api.html#application-tokens](https://docs.taiga.io/api.html#application-tokens).
- The web UI has no settings page for creating personal access tokens.

**Conclusion:** Application Tokens do not replace the password login for the MVP.
They need an admin to create an Application, and they still need a JWT login for the authorization step.
They could be a later option if the Ostfalia admins register a "Taiga TUI" Application.

## 4. What the instance reveals without login

| Probe | Result | Meaning |
|---|---|---|
| `GET /conf.json` | `defaultLoginEnabled: true`, `publicRegisterEnabled: true`, `contribPlugins: []`, no `loginFormType`, `eventsUrl: wss://taiga-fki.ostfalia.de/events` | `normal` login; no SSO/LDAP frontend plugin; public registration allowed; taiga-events is deployed |
| `GET /` | static asset path `v-1778767629306` (build 2026-05-14 14:07 UTC) | this path exists only in [taiga-front-dist tag 6.10.3](https://github.com/taigaio/taiga-front-dist/tree/6.10.3/dist) (6.10.2 has `v-1778158169831`) |
| `GET /v-1778767629306/js/app.js` | SHA-1 `7c456f3b...` equals upstream `6.10.3/dist/v-1778767629306/js/app.js` | **frontend is exactly taiga-front 6.10.3, unmodified** |
| `GET /api/v1/` | 83 routes, including `swimlanes`, `application-tokens`, `user-project-settings` | backend at least 6.0 (swimlanes) |
| `GET /api/v1/auth/refresh` | `405 Method Not Allowed` (route exists) | backend at least 6.3 (simplejwt) |
| `GET /api/v1/applications` | 404 (list route not registered, as in upstream) | matches upstream |
| `GET /api/v1/stats/discover` | `{"projects": {"total": 16}}` | 16 public projects |
| `GET /api/v1/projects` (anonymous) | 200, public projects with `anon_permissions` incl. `view_us`, `view_tasks`, `view_milestones` | public projects can be read without login |
| `GET /api/v1/users/me` | `401 NotAuthenticated` | as expected |
| Response headers | backend `gunicorn`, proxy `nginx/1.23.4`, `Access-Control-Allow-Origin: *` | official docker setup |

The backend does not report its version on any public endpoint.
taiga-docker ships matching back and front versions, so the backend is most likely 6.10.x.
The latest backend tag is 6.10.2.
For the TUI this has no practical impact, because the auth API is unchanged since 6.3.

## 5. Recommended auth flow for the TUI

1. On first start, ask for username (or e-mail) and password in the terminal, with no echo for the password.
   Never accept the password as a CLI flag.
2. `POST /api/v1/auth` with `{"type":"normal","username":...,"password":...}`.
   Keep `id`, `username` and `full_name_display` from the response (the TUI needs its own user id for Assign to me and the "only mine" filter).
   Discard the password right away.
3. Store only the `refresh` token in the OS keychain (macOS Keychain, Secret Service on Linux, Windows Credential Manager).
   Keep `auth_token` in memory only.
4. Send `Authorization: Bearer <auth_token>` on every request.
5. Refresh proactively when `exp` of the access token is close (decode the JWT payload, no signature check needed on the client), and reactively on `401`.
   Store the rotated `refresh` atomically, and make sure only one refresh runs at a time (parallel refreshes with the same token race against the denylist).
6. If refresh returns `401 token_not_valid` (more than 8 days without use, or the token was denylisted), go back to step 1.
7. Make the type a config value (default `"normal"`) only if a second instance ever needs `ldap`.
   For Ostfalia it is not needed.

## 6. HITL login check for ticket #8

Requirements: `curl` and `jq` 1.6 or newer (macOS: `brew install jq`; on Windows use Git Bash or WSL).
Works in bash and zsh.
Credentials come from environment variables and are passed to `curl` through stdin, so they never appear in the process list or shell history.
The output does not contain tokens or the e-mail address, so it can be shared in the issue.
The script was tested in bash and zsh against a local mock server, including a password with quotes, spaces and backslashes.

Step 1, set the credentials (replace `USERNAME`; the password is read without echo):

```sh
export TAIGA_USERNAME='USERNAME'
read -rs TAIGA_PASSWORD && export TAIGA_PASSWORD
```

Step 2, paste this block into the same shell:

```sh
TAIGA_URL="${TAIGA_URL:-https://taiga-fki.ostfalia.de}"
JQ_CLAIMS='def claims: split(".")[1] | gsub("-";"+") | gsub("_";"/") as $s
  | ($s + ("===="[0:((4 - ($s|length) % 4) % 4)])) | @base64d | fromjson;'

# 1) Login (type "normal", same request the web UI sends). Credentials come from env vars via jq env, never argv.
TAIGA_AUTH_RESPONSE=$(jq -n '{type: "normal", username: env.TAIGA_USERNAME, password: env.TAIGA_PASSWORD}' \
  | curl -sS -X POST "$TAIGA_URL/api/v1/auth" \
      -H 'Content-Type: application/json' --data-binary @-)

# 2) Print a redacted summary: no tokens, no email.
printf '%s' "$TAIGA_AUTH_RESPONSE" | jq "$JQ_CLAIMS"'
  if .auth_token then
    {id, username, full_name_display, is_active, roles, response_keys: keys,
     access:  (.auth_token | claims | {token_type, exp: (.exp | todate)}),
     refresh: (.refresh    | claims | {token_type, exp: (.exp | todate)})}
  else . end'

# 3) List the projects I am a member of (slug for the team project). Token goes via stdin header, not argv.
TAIGA_ME=$(printf '%s' "$TAIGA_AUTH_RESPONSE" | jq -r '.id // empty')
if [ -n "$TAIGA_ME" ]; then
  printf '%s' "$TAIGA_AUTH_RESPONSE" | jq -r '"Authorization: Bearer " + .auth_token' \
    | curl -sS -H @- "$TAIGA_URL/api/v1/projects?member=$TAIGA_ME" \
    | jq '[.[] | {id, slug, name, is_backlog_activated, is_kanban_activated}]'
fi

# 4) Forget secrets.
unset TAIGA_AUTH_RESPONSE TAIGA_PASSWORD TAIGA_ME
```

How to read the result:

- Success: a JSON object with `id`, `username`, `response_keys`, and `access.exp` / `refresh.exp`.
  The time from now to `access.exp` is the real access token lifetime (upstream default 24 h); `refresh.exp` gives the refresh lifetime (default 8 days).
  Then a list of the Team Member's projects with their `slug`.
- `{"_error_message": "No active account found with the given credentials", ...}`: the `normal` login does not accept these credentials.
  Then check whether the same credentials work in the web UI.
  If they do, the instance uses a non-upstream login path and #2 must be reopened.
- `{"_error_message": "invalid login type", ...}`: cannot happen with `normal`, unless the backend is patched.

Lowest-common-denominator variant without `jq` (prints the raw response including tokens, so **do not paste it into the issue**):

```sh
curl -sS -X POST https://taiga-fki.ostfalia.de/api/v1/auth \
  -H 'Content-Type: application/json' \
  --data-binary @- <<EOF
{"type":"normal","username":"$TAIGA_USERNAME","password":"$TAIGA_PASSWORD"}
EOF
```

This variant breaks if the password contains `"` or `\`.
