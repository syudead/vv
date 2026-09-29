# Data model: External API and MCP

Parent Issue: #493.

| Source of truth | Location |
| --- | --- |
| Existing table definitions | [internal/store/migrations/](../../internal/store/migrations/) |
| Data classes | [ARCHITECTURE.md](../../ARCHITECTURE.md) "Rebuildable and user data" |
| Rules for `account` and `sessions` | [016 data-model.md](../016-single-account-auth/data-model.md) |

This document covers only the table this feature adds and the rules that read and write it. Tables
not listed here do not change. `videos` gets no new column either; the video list reads the current
columns only ([R-6](research.md#r-6-the-video-list-is-read-with-an-added_at-id-keyset-cursor)).

## 1. `api_tokens` (R-1, R-2, R-9, R-10)

```sql
create table api_tokens (
    id              integer primary key autoincrement,
    -- Purpose name. The value after domain.NormalizeAPITokenName. Duplicates are allowed.
    name            text    not null,
    -- SHA-256 of the plaintext (hex). The plaintext is stored nowhere.
    token_hash      text    not null unique,
    -- account.version at issue time. A row that does not match is invalid (R-2).
    account_version integer not null,
    created_at      integer not null,
    -- Last use time (Unix seconds). null when never used. Not written more often than every 60 s (R-9).
    last_used_at    integer
);
```

`id` has `autoincrement` so that a revoked row's id is never reused by a new token. A revoke request
from the UI therefore never deletes a different token with the same id.

Class: configuration data that cannot be rebuilt (a scan does not restore it; if lost, tokens are
issued again). Add it to the list in ARCHITECTURE.md.

| Operation | Done in one transaction |
| --- | --- |
| Issue (Cookie owner) | Write the read `account.version` to `account_version`, insert one row and return the plaintext. Fails when the `account` row does not exist |
| List | Return `id`, `name`, `created_at` and `last_used_at` in descending `created_at` order. Rows with a mismatched version do not exist (the change below deletes them), but they are not returned if present |
| Revoke | Delete the row with `id`. Does nothing when it does not exist |
| Verify (Bearer) | Look up by the SHA-256 of the plaintext. Only a row with an existing `account` and a matching version is valid. A query failure is an error, not an invalid token |
| Last use time | Write `now` only when `last_used_at is null or last_used_at <= now - 60` |
| Username or password change | Add `delete from api_tokens` to the current `changeCredentials` |

## 2. Values added to `internal/domain`

| Value | Content |
| --- | --- |
| `APIToken` | `ID`, `Name`, `CreatedAt`, `LastUsedAt` (zero value when never used). Holds neither the plaintext nor the hash |
| `NormalizeAPITokenName` and `APITokenNameMaxLength = 100` | The R-10 rule and its error value (same shape as `InvalidTagNameError` for tag names) |
| `VideoRef` | Exactly one of `ID`, `ContentKey` and `Path`. `Path` is not normalized and is compared byte for byte with the location `path` |
| `ExternalVideoQuery`, `ExternalVideoPage` | Cursor and limit; items and `NextCursor` ([R-6](research.md#r-6-the-video-list-is-read-with-an-added_at-id-keyset-cursor)). The only order is ascending `(added_at, id)`; there is no order parameter |
| `VideoTagsAction` | `add`, `remove`, `replace` |
| Error for a video that cannot be found | Wraps `ErrNotFound` and holds the position (from 0) of the `VideoRef` |
