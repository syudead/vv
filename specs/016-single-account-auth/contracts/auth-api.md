# Contract: authentication HTTP boundary

Parent Issue: #135.

[api/openapi.yaml](../../../api/openapi.yaml) is the source of truth for the API. This document
covers only the routes, responses and cookies this feature adds, and the authentication
boundary over every existing route. The guest response differences and the visibility-flag
API are in [guest-api.md](guest-api.md). The implementation adds them to `openapi.yaml` and
generates code with `task generate`.

## 1. Three access classes

Every request falls into one of three classes. A request not listed is "owner only"
(deny by default, Requirement 10).

| Class | Requests | `security` in `openapi.yaml` |
| --- | --- | --- |
| Anyone | `GET /api/health`, `GET /api/auth/session`, `POST /api/auth/setup`, `POST /api/auth/login`, `POST /api/auth/logout`, and `GET`/`HEAD` outside `/api/` (the SPA build output) | `[]` |
| Guests too | `listVideos`, `getVideo`, `getRelatedVideos`, `streamVideo`, `getVideoPreview`, `transcodeVideo`, `getVideoThumbnail`, `getVideoSeekThumbnail`, `listRootFolders`, `getFolder`, `listFolderVideos` | `[{sessionCookie: []}, {}]` |
| Owner only | Every other `/api/*` request, including undefined routes. This includes `listVideoIds`, `streamEvents`, `getProcessing`, scans, media folders, the directory picker, tags, `updateVideoTags`, `putVideoProgress`, `reprobeVideo`, `openVideoFile` and `updateVideoVisibility` | The global default `[{sessionCookie: []}]` |

- A "guests too" request runs as the owner with a valid session, and as a guest without one.
  Guest responses follow [guest-api.md](guest-api.md).
- An "owner only" request without a valid session gets the unauthenticated response of §5.
- While the account is not configured, every request outside "anyone" gets the
  unauthenticated response of §5. The visibility flag has no effect either (Requirement 2).
- The boundary is decided on `r.URL.Path` (decoded) after `path.Clean`. Forms such as
  `//api/…`, `/./api/…`, `/%61pi/…` and `/api/../api/…` count as under `/api/`.
- The SPA build output contains no user data. The SPA chooses what to show from the state
  in §4.
- A Go test checks that each operation's `security` in `openapi.yaml` matches the boundary
  class. `sessionCookie` is declared as an `apiKey` with `in: cookie`.

## 2. `POST /api/auth/setup`

Request (`Content-Type: application/json`, body up to 8 KiB):

```json
{ "username": "string", "password": "string" }
```

| Situation | Response |
| --- | --- |
| Not configured, and the values satisfy [data-model.md §6](../data-model.md#6-username-and-password-values) | `200` `{ "redirectTo": "/" }` with `Set-Cookie` (§7). The client is signed in at once |
| A value breaks the rules | `400` `invalid_request`. The `message` may name the field; no account exists yet, so there is nothing to hide |
| Already configured, including losing a concurrent first setup | `409` `{ "code": "account_already_configured", "message": "The account is already set up." }`. Nothing is written |
| Not same-origin, or not JSON | `403` or `400`, as today |

- The UI compares the confirmation password and does not send the request on a mismatch
  (Requirement 5). The server receives one password only.
- The primary-key conflict in [data-model.md §5](../data-model.md#5-write-rules) lets only
  one setup succeed.

## 3. `POST /api/auth/login`

Request (`Content-Type: application/json`, body up to 8 KiB):

```json
{ "username": "string", "password": "string", "next": "/videos/12?t=30" }
```

`next` is optional.

| Situation | Response |
| --- | --- |
| Success | `200` `{ "redirectTo": "<safe destination>" }` with `Set-Cookie` (§7) |
| Wrong username or password, empty value, value over the limit, or account not configured | `401` `{ "code": "invalid_credentials", "message": "Incorrect username or password." }` |
| The source has 5 or more "verifications in progress + failures in the last 5 min", or no verification slot frees up within 5 s | `429` `{ "code": "login_throttled", … }` with `Retry-After` (seconds) |
| The body is not JSON, exceeds 8 KiB, or is not same-origin | `400` or `403`, as today |

- The four causes of 401 share the status code, body and headers. To equalize response
  time, every cause runs one Argon2id verification; an unconfigured account verifies against
  a fixed dummy hash. The username comparison runs in constant time (Requirement 12).
- Before verifying, the server reserves one attempt for the source. Reservations count
  toward the limit, so concurrent requests never exceed 5 verifications in 5 min.
- A 429 skips verification and does not count as a failure. A success clears the source's
  failure record.
- The source is the client IP address. Behind a trusted proxy, it comes from the proxy's
  forwarding header. IPv6 addresses count as one source per /64.
- `redirectTo` is `next` after validation by the `domain` rules below. A value that breaks
  them, or a missing value, becomes `/`. The UI only navigates to this value and does not
  validate it.
  - No control character, whitespace or `\` anywhere. Browsers strip tabs and newlines when
    parsing a URL, so `/\t/evil.example` becomes `//evil.example`.
  - Parsed as a URL against a fixed base, the result has no scheme and no host, and is a
    path starting with `/`.
  - The path is not `/login` or `/setup`, and is not under `/api/`.
  - The returned value is rebuilt from the parsed path and query string, not the input.

## 4. `GET /api/auth/session` and `POST /api/auth/logout`

`GET /api/auth/session` returns `200` `{ "state": "owner" | "guest" | "setupRequired" }`.

- `setupRequired` means not configured. It is returned with or without a cookie.
- The username is not returned.
- When called with `next` in the query string, the response also carries `redirectTo`,
  validated by the rules of §3, but only for `owner`. A signed-in UI that opens `/login`
  navigates to this value.

`POST /api/auth/logout` returns `204` and clears every authentication cookie on the request
(`Max-Age=0`).

- Unlike the per-scheme reading of §7, logout reads cookies of both names that arrive.
- An HTTPS request also carries a `vv_session` (without `Secure`) from an HTTP sign-in on the
  same host. Logout over HTTPS therefore also ends the HTTP session.
- An HTTP request never carries `__Host-vv_session`, so logout over HTTP ends only the HTTP
  session. The HTTPS session works only over HTTPS and cannot be read from HTTP routes.
- When a cookie points to a valid session, the server deletes that session and cuts off
  responses in progress for it.
- The same-origin check applies, as for other state changes.

## 5. Unauthenticated and other responses

| Situation | Response |
| --- | --- |
| An "owner only" request with no cookie, a malformed cookie, no matching session, an expired session, a session from before a credential reset, or an unconfigured account | `401` `{ "code": "unauthenticated", "message": "Sign-in required." }` |
| A "guests too" request for a video hidden from guests | The same `404` as the existing "Video not found." ([guest-api.md §2](guest-api.md#2-videos-hidden-from-guests)) |
| The DB fails while checking the session or the visibility flag | `500` `{ "code": "internal", … }`. The request is treated as neither owner nor public |

- 401 does not distinguish causes (Edge case "invalid, expired or tampered cookie").
  `WWW-Authenticate` is not sent, so the browser shows no Basic-auth dialog.
- A "guests too" request with an invalid or expired cookie runs as a guest instead of
  returning 401.
- Every `/api/*` response carries `X-VV-Audience: owner` or `X-VV-Audience: guest`, naming
  how the request ran. While rendering as the owner, the UI reloads the page once on a
  `guest` response, as it does on 401. A "guests too" request never returns 401 after
  expiry, so this header reveals a logout in another tab on the next action (Edge case
  "multiple tabs").
- All of these carry `Cache-Control: no-store` and never return HTML (Requirement 11).
- A DB failure is not a 401. Otherwise the UI would go to the sign-in screen, fail there too,
  and loop.

## 6. `GET /api/health`

The response shape is unchanged (`status`, `version`, `commit`, `builtAt`). It carries no
library content, settings, username, authentication state or first-setup need (Acceptance
criterion 16). `version` and `commit` stay because they identify the running binary.

## 7. Session cookie

| Connection | Name | Attributes |
| --- | --- | --- |
| HTTPS | `__Host-vv_session` | `HttpOnly; Secure; SameSite=Strict; Path=/; Max-Age=<seconds until expiry>` |
| HTTP | `vv_session` | `HttpOnly; SameSite=Strict; Path=/; Max-Age=<seconds until expiry>` |

- The value is the session ID: 32 bytes from a cryptographic RNG, in base64url
  (Requirement 6).
- `Max-Age` is set, so the cookie survives a browser restart until expiry (Acceptance
  criterion 10).
- HTTP uses the same mechanism, with no attribute or API that requires HTTPS
  (Requirement 14).
- For authentication, an HTTPS request reads only `__Host-vv_session`, and an HTTP request
  reads only `vv_session`. Logout is the exception and reads both (§4). The separate names
  keep an HTTP response from overwriting the HTTPS cookie, and keep the HTTPS cookie from
  being sent over HTTP (Edge case "HTTP and HTTPS").
- A connection is HTTPS when it arrived over TLS, or when a trusted proxy's
  `X-Forwarded-Proto` says so.

## 8. Same-origin check

The existing `acceptsSameOrigin` (`internal/httpapi/media_folders.go`) stays as is: it applies
to every `POST`, `PUT`, `PATCH` and `DELETE`, including setup, sign-in and logout
(Requirement 13). The only change is that the expected scheme comes from the same decision as
§7. Today it reads only `r.TLS`, so it rejects same-origin requests behind a TLS-terminating
proxy.

## 9. Logging

Setup, sign-in success, sign-in failure, throttling and logout each log one `slog` line at
`info`.

- The attributes are the event type and the source only. The submitted username, password,
  session ID and cookie value are never logged (Requirement 12).
- The submitted username is withheld because a password typed into the username field would
  otherwise stay in the log in plaintext.
- A start without a configured account logs one warning that asks for the first setup.
