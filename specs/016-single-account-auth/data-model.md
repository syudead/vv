# Data model: single-account authentication and the visibility flag

Parent Issue: #135.

[internal/store/migrations/](../../internal/store/migrations/) is the source of truth for the
existing tables. [ARCHITECTURE.md](../../ARCHITECTURE.md) separates the "rebuildable index"
from "user data that cannot be recreated". This document covers only the three tables this
feature adds and the rules that read and write them. Existing tables do not change.

## 1. Migrations

Two migrations are added. The last migration on `main` is `00009_display_aspect_ratio.sql`.

`00010_auth.sql`:

```sql
-- The only account. No row means "not configured"; a row means "configured" (Requirement 2).
-- Only the first setup creates the row, and it is never deleted afterwards.
create table account (
    id            integer primary key check (id = 1),
    -- Default BINARY collation: an exact, case-sensitive match (Requirement 4).
    username      text    not null,
    -- Argon2id PHC string ($argon2id$v=19$m=…,t=…,p=…$<salt>$<hash>). No plaintext (Requirement 5).
    password_hash text    not null,
    -- Incremented by 1 on every username or password change (§4).
    version       integer not null default 1 check (version >= 1),
    updated_at    integer not null
);

-- Sessions issued by sign-in and the first setup.
create table sessions (
    -- SHA-256 (hex) of the session ID. The ID itself lives only in the cookie, never in the DB.
    token_hash      text    primary key,
    -- account.version at issue time. A row that does not match is invalid (§4).
    account_version integer not null,
    created_at      integer not null,
    -- created_at + 90 days. Never extended (Requirement 7).
    expires_at      integer not null
) without rowid;

create index sessions_expires_at on sessions (expires_at);
```

`00011_public_videos.sql`:

```sql
-- Visibility flag. A row means public; no row means non-public (default) (Requirement 15).
-- Keyed by the content identifier, not the video row, so it survives rescans, moves, renames
-- and media-folder changes. Like playback_progress and video_tags, no foreign key to videos.
create table public_videos (
    content_key  text    primary key,
    published_at integer not null
) without rowid;
```

Each Down migration drops its tables.

## 2. Data classes

| Table | Class | Effect of deletion |
| --- | --- | --- |
| `account` | User data that cannot be recreated | Setup starts over from the first setup |
| `public_videos` | User data that cannot be recreated | Every video becomes non-public again |
| `sessions` | Neither: transient state | Restored by signing in again |

The data-class paragraph of ARCHITECTURE.md gains these three tables.

## 3. Audience and the public-video condition

For each request, the audience is either the "owner" (holds a valid session) or a "guest"
(does not). It lives in `internal/domain` as `Audience`, and its zero value is guest, so a
missing value falls to the narrower side. An empty `content_key` (a video whose content is not
read yet) never enters `public_videos` and is hidden from guests.

A guest sees a video only when both conditions hold:

1. `videos.content_key` is not empty and is in `public_videos`.
2. As in the current lists, the video has a location under a registered media folder.

One function takes an `Audience` and returns the location condition (for example,
`visibleLocationCondition(alias, audience)`). For the owner it returns the current "under a
registered folder" condition unchanged; for a guest it adds condition 1. Every read below,
which builds the condition in place today, goes through this function.

| Read | Where the condition is built today |
| --- | --- |
| `ListVideos`, `ListFolderVideos`, `DirectVideoPaths` | `locationScope.condition` in `listing.go` |
| `GetVideo` | `registeredVideoCondition` and the subquery in `videoColumns` in `videos.go` |
| `VideosAddedNear`, `VideosByIDs` | `related.go` |
| `FolderLocations`, `HasFolderLocations` | `folders.go` |

- The condition functions used by ingest, jobs and tag counts (`videos.go`, `jobs.go`,
  `tags.go`) stay owner-side and never take the guest condition.
- Every `LibraryStore` read that returns videos, locations or folders takes an `Audience`.
- Folders are still derived from location paths. Guests therefore see only folders derived
  from public-video locations, and counts include public videos only.
- When reading the visibility flag fails, the request returns an error and the video is not
  treated as public (Edge case "DB failure on the visibility flag").

## 4. Session validity

A session is valid only when all of the following hold. One query checks them.

1. The SHA-256 of the cookie value is in `sessions.token_hash`.
2. The `account` row exists.
3. `sessions.account_version = account.version`.
4. `expires_at` is later than now.

- Condition 3 exists for a race: a host command in another process may reset the
  credentials between sign-in verification (Argon2id, tens of milliseconds) and the row
  insert. Condition 3 keeps a session verified against the old credentials from surviving.
- Sign-in writes the `account.version` it read for verification into `account_version`
  unchanged.
- When the query fails, the session is not treated as invalid; the query returns an error.
  The HTTP side turns it into 500
  ([contracts/auth-api.md §5](contracts/auth-api.md#5-unauthenticated-and-other-responses)).

## 5. Write rules

| Operation | What one transaction does |
| --- | --- |
| First setup | Inserts the `account` row with `id = 1` and one session row. If the row exists, the primary-key conflict fails the transaction with nothing written, so only one of two concurrent setups succeeds (Requirement 2) |
| Change the username (host command) | Fails when no row exists. Writes `username`, increments `version` by 1 and deletes every row in `sessions` (Requirement 9) |
| Change the password (host command) | Fails when no row exists. Writes `password_hash`, increments `version` by 1 and deletes every row in `sessions` (Requirement 9) |
| Sign-in succeeds | Deletes rows with `expires_at <= now` and inserts a new row. When the request carries a valid session cookie, deletes that row too, so signing in again leaves no old row. The UI prevents a double submit of the first sign-in without a cookie; a row that slips through expires |
| Logout | Deletes the row matching the cookie value. Does nothing when none matches |
| Check validity (§4) | The decision only reads. On hitting an expired row, it tries to delete that row. A failed delete does not change the decision |
| Start | Deletes rows with `expires_at <= now` |
| Make public or non-public | Maps the given video ids to the `content_key` of videos now in the library. Public inserts into `public_videos` (no-op when present); non-public deletes. Applies to all or none, as in tag assignment ([014 tags-api.md §4](../014-video-tags/contracts/tags-api.md#4-adding-and-removing-tags)) |

Deleting every `sessions` row is cleanup. Condition 3 of §4 performs the invalidation itself.

## 6. Username and password values

- Username: 1 to 128 characters, no control characters, no leading or trailing whitespace.
  No normalization and no case folding. The rule lives in `internal/domain` and applies to
  the first setup and the host commands. Sign-in compares the submitted value with the stored
  value byte for byte.
- Password: 1 to 1024 bytes. The parent Issue sets no strength rule, so none exists.

An empty or over-limit value on a sign-in request is not a format error; it is the same
authentication failure as the others
([contracts/auth-api.md §3](contracts/auth-api.md#3-post-apiauthlogin)). On the first setup,
such a value is a format error and nothing is configured.
