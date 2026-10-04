# Data model: External API and MCP

Parent Issue: #493.

The rest of the model is unchanged. Existing table definitions are canonical in
[internal/store/migrations/](../../internal/store/migrations/), the data classes are in
[ARCHITECTURE.md](../../ARCHITECTURE.md) "Rebuildable and user data", and the rules for `account` and
`sessions` are in [016 data-model.md](../016-single-account-auth/data-model.md). This file records only
the table this feature adds and the rules that read and write it. No other table changes (`videos`
gets no column either; the video list is read from the existing columns,
[R-6](research.md#r-6-the-video-list-is-read-with-an-added_at-id-keyset-cursor)).

## 1. `api_tokens` (R-1, R-2, R-9, R-10)

```sql
create table api_tokens (
    id              integer primary key autoincrement,
    -- Purpose name. The value after domain.NormalizeAPITokenName. Duplicates allowed.
    name            text    not null,
    -- SHA-256 (hex) of the plaintext. The plaintext is never stored anywhere.
    token_hash      text    not null unique,
    -- account.version at the time of issue. A row that does not match is invalid (R-2).
    account_version integer not null,
    created_at      integer not null,
    -- Last time used (Unix seconds). null if never used. Not written more often than every 60 seconds (R-9).
    last_used_at    integer
);
```

`id` has `autoincrement` so that the id of a revoked row is never reused for a new token (a revoke
request from the screen never deletes a different token with the same id).

Class: settings data that cannot be rebuilt (a scan does not restore it; if lost, tokens are issued
again). It is added to the list in ARCHITECTURE.md.

| Operation | What one transaction does |
| --- | --- |
| Issue (cookie owner) | Writes the `account.version` it read into `account_version`, adds one row, and returns the plaintext. Fails if there is no `account` row. |
| List | Returns `id`, `name`, `created_at` and `last_used_at` in descending `created_at` order. Rows with a mismatched version should not exist (the change below deletes them), but any that do are not returned. |
| Revoke | Deletes the row with that `id`. Does nothing if it does not exist. |
| Verify (Bearer) | Looks the row up by the SHA-256 of the plaintext; only a row for which `account` exists and the version matches is valid. A query failure is an error, not "invalid". |
| Record use | Writes `now` only when `last_used_at is null or last_used_at <= now - 60`. |
| Change username or password | Adds `delete from api_tokens` to the existing `changeCredentials`. |

## 2. Values added to `internal/domain`

| Value | Contents |
| --- | --- |
| `APIToken` | `ID`, `Name`, `CreatedAt`, `LastUsedAt` (zero value when never used). Holds neither the plaintext nor the hash. |
| `NormalizeAPITokenName` and `APITokenNameMaxLength = 100` | The R-10 rules and their error value (same shape as the tag name's `InvalidTagNameError`). |
| `VideoRef` | Exactly one of `ID`, `ContentKey` and `Path`. `Path` is not normalised; it is compared byte for byte with the location's `path`. |
| `ExternalVideoQuery`, `ExternalVideoPage` | Cursor and limit; items and `NextCursor` ([R-6](research.md#r-6-the-video-list-is-read-with-an-added_at-id-keyset-cursor)). The order is only `(added_at, id)` ascending; there is no order option. |
| `VideoTagsAction` | `add`, `remove`, `replace`. |
| Video-not-found error | Wraps `ErrNotFound` and carries the position of the `VideoRef` (from 0). |
