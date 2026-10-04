# Data model: Single-account authentication and the public flag

Parent Issue: #135.

The existing tables are defined in
[internal/store/migrations/](../../internal/store/migrations/). The distinction
between a rebuildable index and user data that cannot be recreated is in
[ARCHITECTURE.md](../../ARCHITECTURE.md). This document covers only the three
tables this feature adds and the rules for reading and writing them. Existing
tables do not change.

## 1. Migration

Two migrations are added. The last migration on `main` is
`00009_display_aspect_ratio.sql`.

`00010_auth.sql`:

```sql
-- The only account. No row means "not configured", a row means "configured" (requirement 2).
-- Only first-time setup creates the row, and it is never deleted.
create table account (
    id            integer primary key check (id = 1),
    -- Default BINARY collation: an exact, case-sensitive match (requirement 4).
    username      text    not null,
    -- Argon2id PHC string ($argon2id$v=19$m=…,t=…,p=…$<salt>$<hash>). No plain text is kept (requirement 5).
    password_hash text    not null,
    -- Incremented on every username or password change (§4).
    version       integer not null default 1 check (version >= 1),
    updated_at    integer not null
);

-- Sessions issued by login and first-time setup.
create table sessions (
    -- SHA-256 (hex) of the session ID. The ID itself exists only in the cookie, not in the DB.
    token_hash      text    primary key,
    -- account.version at issue time. A row that does not match is invalid (§4).
    account_version integer not null,
    created_at      integer not null,
    -- created_at + 90 days. Not extended (requirement 7).
    expires_at      integer not null
) without rowid;

create index sessions_expires_at on sessions (expires_at);
```

`00011_public_videos.sql`:

```sql
-- Public flag. A row means public, no row means private (the default) (requirement 15).
-- Keyed by the content identifier rather than the video row, so rescans, moves,
-- renames and media folder changes do not lose it. Like playback_progress and
-- video_tags, there is no foreign key to videos.
create table public_videos (
    content_key  text    primary key,
    published_at integer not null
) without rowid;
```

Down drops each table.

## 2. Data classification

| Table | Class | When lost |
| --- | --- | --- |
| `account` | User data that cannot be recreated | Setup starts over from first-time setup |
| `public_videos` | User data that cannot be recreated | Every video returns to private |
| `sessions` | Temporary state, in neither class | Restored by logging in again |

The classification paragraph in ARCHITECTURE.md gains these three tables.

## 3. Audience and the public video condition

For each request the audience is either the owner (has a valid session) or a
guest (does not). It lives in `internal/domain` as `Audience`, whose zero value
is the guest, so forgetting to set it falls to the narrower side. An empty
`content_key` (a video whose content has not been read yet) is never put in
`public_videos` and is never shown to guests.

Guests see only videos that satisfy both:

1. `videos.content_key` is not empty and is in `public_videos`.
2. As in the current list, a location is under a registered media folder.

This condition lives in one function that takes an `Audience` and returns the
location condition (for example
`visibleLocationCondition(alias, audience)`). For the owner it returns the
current "under a registered folder" condition unchanged; for a guest it adds 1.
Every read below, which today builds its own condition, goes through this
function.

| Read | Where the condition is today |
| --- | --- |
| `ListVideos`, `ListFolderVideos`, `DirectVideoPaths` | `locationScope.condition` in `listing.go` |
| `GetVideo` | `registeredVideoCondition` and the subquery in `videoColumns` in `videos.go` |
| `VideosAddedNear`, `VideosByIDs` | `related.go` |
| `FolderLocations`, `HasFolderLocations` | `folders.go` |

The condition functions used by import, jobs and tag counts (`videos.go`,
`jobs.go`, `tags.go`) stay owner-only and do not take the guest condition.

Every `LibraryStore` read that returns videos, locations or folders takes an
`Audience` argument. Folders are still derived from location paths, so guests
see only folders derived from locations of public videos, and counts include
only public videos.

When reading the public flag fails, the request returns an error and the video
is not treated as public (Edge Case `公開フラグの DB の失敗`).

## 4. Session validity conditions

A session is valid only when all of these hold, checked in one query:

1. The SHA-256 of the cookie value is in `sessions.token_hash`.
2. The `account` row exists.
3. `sessions.account_version = account.version`.
4. `expires_at` is later than now.

Condition 3 exists so that, if a command in another process resets the
credentials between the login check (Argon2id, tens of milliseconds) and the row
insert, a session checked against the old credentials does not survive. Login
writes the `account.version` it read for the check into `account_version`
unchanged.

When the query fails, the result is an error, not "invalid". The HTTP side
turns it into 500
([contracts/auth-api.md §5](contracts/auth-api.md#5-unauthenticated-and-other-responses)).

## 5. Write rules

| Operation | What one transaction does |
| --- | --- |
| First-time setup | Insert the `account` row with `id = 1` and one session row. When the row already exists, the primary key collision makes it fail without writing (only one concurrent first-time setup succeeds, requirement 2) |
| Change the username (host command) | Fail when there is no row. Write `username`, increment `version`, delete every row in `sessions` (requirement 9) |
| Change the password (host command) | Fail when there is no row. Write `password_hash`, increment `version`, delete every row in `sessions` (requirement 9) |
| Log in successfully | Delete rows with `expires_at <= now` and insert a new row. When the request carries a cookie for a valid session, delete that row too (so logging in again leaves no old row; the screen prevents a double submit of a first login without a cookie, and any row that slips through expires) |
| Log out | Delete the row matching the cookie value; do nothing when there is none |
| Check validity (§4) | The decision is made by reading only. On hitting an expired row, try to delete it; a failed delete does not change the decision |
| Start up | Delete rows with `expires_at <= now` |
| Make public or private | Resolve the given video ids to the `content_key`s of videos currently in the library; for public, insert into `public_videos` (nothing when present); for private, delete. Applies to all or none (as adding and removing tags, [014 tags-api.md §4](../014-video-tags/contracts/tags-api.md#4-adding-and-removing-tags-on-videos)) |

Deleting every row in `sessions` is cleanup; invalidation itself is done by
condition 3 of §4.

## 6. Username and password values

| Value | Rule |
| --- | --- |
| Username | 1 to 128 characters. No control characters, no leading or trailing whitespace. No normalization or case folding. The rule lives in `internal/domain` and applies to first-time setup and the host command. Login compares the submitted value byte for byte with the stored value |
| Password | 1 to 1024 bytes. The parent Issue has no strength rule, so none is set |

An empty or over-limit value in a login request is not a format error; it is
the same authentication failure as any other
([contracts/auth-api.md §3](contracts/auth-api.md#3-post-apiauthlogin)). In
first-time setup it is a format error and nothing is configured.
