# Contract: Authentication HTTP boundary

Parent Issue: #135.

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). This document
covers only the routes, responses and cookies this feature adds, and the
authentication boundary over every existing route. Differences in what guests
receive, and the public flag API, are in [guest-api.md](guest-api.md). The
implementation adds them to `openapi.yaml` and generates code with
`task generate`.

## 1. Three access classes

Every request falls into one of three classes. Anything not listed is "Owner
only" (deny by default, requirement 10).

| Class | Requests | `security` in `openapi.yaml` |
| --- | --- | --- |
| Anyone | `GET /api/health`, `GET /api/auth/session`, `POST /api/auth/setup`, `POST /api/auth/login`, `POST /api/auth/logout`, and `GET` and `HEAD` not under `/api/` (the SPA build output) | `[]` |
| Guest too | `listVideos`, `getVideo`, `getRelatedVideos`, `streamVideo`, `getVideoPreview`, `transcodeVideo`, `getVideoThumbnail`, `getVideoSeekThumbnail`, `listRootFolders`, `getFolder`, `listFolderVideos` | `[{sessionCookie: []}, {}]` |
| Owner only | Every other `/api/*`, including undefined routes. This includes `listVideoIds`, `streamEvents`, `getProcessing`, scans, media folders, directory picking, tags, `updateVideoTags`, `putVideoProgress`, `reprobeVideo`, `openVideoFile` and `updateVideoVisibility` | The global default `[{sessionCookie: []}]` |

- A "Guest too" request is handled as the owner when it has a valid session, and
  as a guest otherwise. Responses handled as a guest follow
  [guest-api.md](guest-api.md).
- An "Owner only" request without a valid session gets the unauthenticated
  response of §5.
- While the account is not configured, every request except "Anyone" gets the
  unauthenticated response of §5 (the public flag has no effect either,
  requirement 2).
- The boundary is decided on `r.URL.Path` (decoded) after `path.Clean`. Forms
  such as `//api/…`, `/./api/…`, `/%61pi/…` and `/api/../api/…` count as under
  `/api/`.
- The SPA build output contains no user data. The SPA chooses what to show from
  the state in §4.
- A Go test checks that each operation's `security` in `openapi.yaml` matches
  its boundary class. `sessionCookie` is declared as an `apiKey` with
  `in: cookie`.

## 2. `POST /api/auth/setup`

Request (`Content-Type: application/json`, body up to 8 KiB):

```json
{ "username": "string", "password": "string" }
```

| Situation | Response |
| --- | --- |
| Not configured, and the values satisfy [data-model.md §6](../data-model.md#6-username-and-password-values) | `200` `{ "redirectTo": "/" }` with `Set-Cookie` (§7). The user is logged in |
| The values break the rules | `400` `invalid_request` (`message` may name the field; with no account yet there is nothing to hide) |
| Already configured (including losing a concurrent first-time setup) | `409` `{ "code": "account_already_configured", "message": "アカウントは既に設定されています" }`. Nothing is written |
| Not same-origin, not JSON | `403`, `400` as now |

- The screen compares the confirmation password and does not send on mismatch
  (requirement 5). The server receives one password.
- Which setup succeeds is decided by the primary key collision in
  [data-model.md §5](../data-model.md#5-write-rules).

## 3. `POST /api/auth/login`

Request (`Content-Type: application/json`, body up to 8 KiB):

```json
{ "username": "string", "password": "string", "next": "/videos/12?t=30" }
```

`next` is optional.

| Situation | Response |
| --- | --- |
| Success | `200` `{ "redirectTo": "<safe return target>" }` with `Set-Cookie` (§7) |
| Wrong, empty or over-limit username or password, or the account not configured | `401` `{ "code": "invalid_credentials", "message": "ユーザー名またはパスワードが違います" }` |
| The source's "in-progress checks + failures in the last 5 minutes" is 5 or more, or no check slot frees up within 5 seconds | `429` `{ "code": "login_throttled", … }` with `Retry-After` (seconds) |
| The body is not JSON, exceeds 8 KiB, or the request is not same-origin | `400`, `403` as now |

- The four causes of 401 share the status code, body and headers. To even out
  response time, every cause runs one Argon2id check (against a fixed dummy hash
  when not configured). The username is compared in constant time
  (requirement 12).
- Before the check, one attempt is reserved for the source. Reservations count
  toward the limit, so even concurrent requests do not exceed 5 checks in 5
  minutes.
- A 429 runs no check and does not count as a failure. Success clears the
  source's failure record.
- The source is the client IP address, taken from the forwarding header when
  the request came through a trusted proxy. IPv6 addresses count as one source
  per /64.
- `redirectTo` is `next` validated by the `domain` rules below; anything that
  fails, or an omitted `next`, becomes `/`. The screen only navigates to this
  value and does not judge it.
  - It contains no control character, whitespace or `\` anywhere (browsers
    strip tabs and newlines when parsing URLs, so `/\t/evil.example` becomes
    `//evil.example`).
  - Parsed as a URL against a fixed base, it has no scheme or host and is a
    path starting with `/`.
  - The path is not `/login`, `/setup`, or under `/api/`.
  - The returned value is the parsed path and query rebuilt, not the input
    itself.

## 4. `GET /api/auth/session` and `POST /api/auth/logout`

`GET /api/auth/session` returns `200` `{ "state": "owner" | "guest" | "setupRequired" }`.

- `setupRequired` means not configured and is returned whether or not a cookie
  is present.
- The username is not returned.
- Called with a `next` query parameter, it also returns `redirectTo`, validated
  by the rules of §3, when the state is `owner`. A logged-in screen that opened
  `/login` navigates to this value.

`POST /api/auth/logout` returns `204` and clears every authentication cookie
the request carried (`Max-Age=0`). Unlike the per-scheme reading in §7, logout
looks at cookies of both names that arrive:

| Logout over | Cookies that arrive | Sessions ended |
| --- | --- | --- |
| HTTPS | `__Host-vv_session` and `vv_session` (without `Secure`) from an HTTP login on the same host | Both the HTTPS and the HTTP session |
| HTTP | Only `vv_session`; `__Host-vv_session` is not sent | Only the HTTP session. The HTTPS session is usable only over HTTPS and cannot be read from HTTP routes |

When an arriving cookie points at a valid session, that session is deleted and
responses in progress for it are cut off. The same-origin check applies as for
other state changes.

## 5. Unauthenticated and other responses

| Situation | Response |
| --- | --- |
| An "Owner only" request with no cookie, a malformed cookie, no matching session, an expired session, a session from before a credential reset, or the account not configured | `401` `{ "code": "unauthenticated", "message": "ログインが必要です" }` |
| A "Guest too" request pointing at a video hidden from guests | The same `404` as the existing "no such video" ([guest-api.md §2](guest-api.md#2-videos-hidden-from-guests)) |
| The DB fails while checking the session or the public flag | `500` `{ "code": "internal", … }` (treated as neither owner nor public) |

- 401 causes are not told apart (Edge Case `不正、期限切れ、改ざん済みの Cookie`).
  No `WWW-Authenticate` is sent, so the browser shows no Basic authentication
  dialog.
- A "Guest too" request with an invalid or expired cookie is handled as a guest,
  not as 401.
- Every `/api/*` response carries `X-VV-Audience: owner` or
  `X-VV-Audience: guest`, saying how the request was handled. A screen drawn as
  the owner that receives a `guest` response reloads the page once, as for
  401. "Guest too" requests do not return 401 after the session ends, so this
  is how a logout in another tab is noticed at the next action (Edge Case
  `複数タブ`).
- All of these are `Cache-Control: no-store` and never return HTML
  (requirement 11).
- A DB failure is not 401, so the screen does not send the user to the login
  screen only to fail there too.

## 6. `GET /api/health`

The response shape does not change (`status`, `version`, `commit`, `builtAt`).
It carries no library content, settings, username, authentication state, or
whether first-time setup is needed (acceptance criterion 16). `version` and
`commit` stay because they identify the running binary.

## 7. Session cookie

| Connection | Name | Attributes |
| --- | --- | --- |
| HTTPS | `__Host-vv_session` | `HttpOnly; Secure; SameSite=Strict; Path=/; Max-Age=<seconds until expiry>` |
| HTTP | `vv_session` | `HttpOnly; SameSite=Strict; Path=/; Max-Age=<seconds until expiry>` |

- The value is the session ID (base64url of 32 cryptographically random bytes)
  (requirement 6).
- `Max-Age` is set, so the cookie survives closing the browser until expiry
  (acceptance criterion 10).
- HTTP works the same way and uses no attribute or API that assumes HTTPS
  (requirement 14).
- For authentication, an HTTPS request reads only `__Host-vv_session` and an
  HTTP request only `vv_session` (logout is the exception and looks at both, as
  in §4). With separate names, an HTTP response never overwrites the HTTPS
  cookie and the HTTPS cookie is never sent over HTTP (Edge Case
  `HTTP と HTTPS`).
- Whether a connection is HTTPS is decided by whether it arrived over TLS, or by
  `X-Forwarded-Proto` from a trusted proxy.

## 8. Same-origin check

The existing `acceptsSameOrigin` (`internal/httpapi/media_folders.go`) keeps
its rule of applying to every `POST`, `PUT`, `PATCH` and `DELETE`, including
first-time setup, login and logout (requirement 13). The only change is that the
expected scheme comes from the same decision as §7. Today it looks only at
`r.TLS`, so behind a TLS-terminating proxy it rejects same-origin requests too.

## 9. Logging

First-time setup, login success, login failure, throttling and logout each log
one `slog` line at `info`. Attributes are only the event type and the source;
the submitted username, password, session ID and cookie values are never logged
(requirement 12). The submitted username is excluded because a password typed
into the username field by mistake would otherwise be logged in plain text.
Starting with the account not configured logs one warning line prompting
first-time setup.
