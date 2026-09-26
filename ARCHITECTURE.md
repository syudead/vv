# Architecture

This repository targets a self-hosted media data management (MDM) system: it
indexes video files on local storage and plays them back in a browser. The
core is in place — a single Go binary that scans configured media folders, serves the
JSON API and the embedded React SPA, streams video with byte ranges, and
records playback positions, backed by SQLite — and the remaining components are
introduced as later phases land. The selected stack and the component
boundaries are recorded in
[docs/design-docs/tech-stack-selection.md](docs/design-docs/tech-stack-selection.md).
Expand the sections below as implementation lands.

## Intended topology

A single Go binary serves the JSON API, the embedded React SPA build, and
byte-range video streaming (via `http.ServeContent`), backed by SQLite and by
video files on a mounted volume. `ffmpeg`/`ffprobe` run as child processes for
metadata, thumbnails, and subtitle conversion, driven by an in-process job
worker. Everything ships as one container.

In place today: `cmd/mdm` reads the remaining `MDM_*` environment variables, checks that
`ffprobe`/`ffmpeg` are on `PATH`, opens SQLite under `MDM_DATA_DIR` and applies
embedded goose migrations at startup, then starts the job worker. It serves `GET /api/health`,
the video library API (`/api/videos*`, `/api/scans*`; a single video's response also
carries its representative location, the folder that holds it (with the registered folder's
display name, for the playback page's breadcrumb), seek-preview state and, for a folder-group
member, the group and its position in it, and
`/api/videos/{id}/related`, `/probe` and `/open` return related videos (for a group member,
also every member in group order, with next/previous inside the group), retry a failed
metadata read, and open the file in the server PC's default app), media-folder settings and
server-side directory picker APIs, the read-only folder browsing API
(`/api/folders*`), the tag management API (`/api/tags*`: list, create,
rename, delete, merge and synonym registration/removal), the video-tags API
(`/api/video-tags` to attach/detach a tag on a set of videos and
`/api/video-tags/summary` to summarize which tags apply to a selection),
the library items API (`/api/library*`, below), byte-range streaming,
thumbnails, playback progress, and the SPA embedded from `web/dist`.

The per-video lists, `GET /api/videos` (the folder view's root search) and a folder
(`GET /api/folders/{rootId}/videos`, direct children by default or the whole
subtree with `scope=subtree`), accept the same search expression (`query`),
watch-state and playable filters, thirteen sort orders and a shuffle `seed`.
`internal/httpapi` validates those parameters at the entry and hands them to
the store as `domain.VideoQuery` / `domain.FolderVideoQuery`; `total` counts
every match after all of them apply, and each item carries the folder of the
location it was listed from (`Video.folder`, built from the registered media
folders with `domain.LocateVideoFolder`)
([specs/013-library-search/contracts/list-api.md](specs/013-library-search/contracts/list-api.md)).
Every `Video` response (list, single, related, and retried-probe) also carries
`tags` (an empty array when none), looked up in `internal/httpapi` from
`TagStore.TagsByContentKeys` the same way `progressFor` looks up playback
positions. The library list additionally accepts up to 16 `tag` ids (AND) and
reports any that no longer exist in `missingTagIds`
([specs/014-video-tags/contracts/tags-api.md](specs/014-video-tags/contracts/tags-api.md)).

`GET /api/library` takes the same parameters as `GET /api/videos` but returns
library items: a video, or a folder group as one item
(`LibraryStore.ListLibrary`). The search, playable and tag filters apply per
member, a group appears when any member matches, its values (count, total
duration and size, latest dates, watch state and the member to open, decided by
`domain.GroupWatch` and `domain.GroupOpenIndex`) come from all of its members,
and the watch filter, sort and `total` apply to items. `GET /api/library/ids`
returns the matched videos plus every member of matched groups (owner only),
and `GET /api/folders/{rootId}/group` refetches one group card. A guest sees
groups built from public members only; a group with one public member is listed
as that video
([specs/017-folder-groups/contracts/library-api.md](specs/017-folder-groups/contracts/library-api.md)).
`GET /api/videos` stays a per-video list for the folder view's root search.

`internal/scanner` walks a snapshot of the media folders stored in SQLite when a user starts
a scan. It identifies files by content
(`sha256` over the first and last 1MiB plus the size) so moves and renames do
not duplicate rows, and queues the heavy work. Only a user-started scan walks the
media folders. Just before a scan closes (done or failed), `internal/app` asks
`ScanIndexStore.RebuildFolderIndex` to rebuild the folder index — folder groups and each
video's ancestor folder names — by reading every location row in SQLite, never the
filesystem; a failed rebuild does not fail the scan but marks the index stale. At startup
the folder index is rebuilt only when its rule version is out of date or it is stale
(`RefreshFolderIndex`, after the search-key refresh and before HTTP and the workers
start), or when an interrupted scan was closed
([specs/017-folder-groups/data-model.md](specs/017-folder-groups/data-model.md) §3).
`internal/jobs` runs one in-process worker per ingest stage — probe, thumbnail, preview —
each claiming only its own kind of job from the persistent `jobs` queue, one at a time,
and handing it to `internal/app`, which drives the `internal/media` adapters
(`ffprobe` for metadata, `ffmpeg` for one library thumbnail, five-second seek-preview frames,
and a content-keyed hover-preview clip per video) and publishes their output through
`internal/artifacts`. A worker sleeps while its queue is
empty: `internal/store` publishes `domain.JobsQueued` after every committed enqueue, and a
subscription wakes the worker for that stage, so no worker polls the queue. A thumbnail job
is not claimed until its video's probe has finished, because the frame position depends on
the duration; `internal/app` publishes `domain.VideoIngestChanged` with the finished stage,
and a subscription wakes the thumbnail worker as soon as a probe's result is recorded. Interrupted scans are closed,
running jobs are requeued, and the single `.tmp` directory that holds in-progress
generation output is removed at the next startup. When a video row is deleted (a scan finds its last
location gone, its content changes, or its media folder is removed or replaced),
`internal/store` publishes the released content keys (`domain.ContentUnreferenced`) after
commit, and `internal/app`, subscribed to that event, removes that content's thumbnail, seek frames and hover preview unless another video still
references it; nothing else sweeps the thumbnails directory. A hover preview that is gone
or incomplete is repaired when it is found: `internal/app` already checks it before the
video API exposes `previewUrl`, and when a `done` preview's MP4 is missing or does not match
the size in its manifest it sets the video back to `pending` and queues a preview job in one
transaction, once per loss.

Generated files have one owner, `internal/artifacts`. Under `MDM_DATA_DIR/thumbnails`
(the root comes from `cmd/mdm`'s configuration) it alone decides where each content key's
files live — the library thumbnail at `<p>/<s>.jpg`, the seek frames under `seek/<p>/<s>/`,
the hover preview and its size/SHA-256 manifest at `preview/<p>/<s>.mp4[.sha256]`, where
`<s>` is the content key with `:`, `/` and `\` replaced by `_` and `<p>` its first two
characters — and it alone creates, checks, opens and removes them. Generation writes into
a directory under `.tmp` that `internal/artifacts` hands out and then renames into place,
so a file still being generated is neither reported as present nor served. A content key
that would point outside the root (empty, or starting with `.`) never becomes a path.
`internal/media` only runs `ffmpeg` against the output path it is given; `internal/app`
(deciding when to generate and when to remove, under its per-content lock) and
`internal/httpapi` (serving the files) reach the store through interfaces they declare.
Changing that layout would orphan every file an existing data directory already holds.

State changes that trigger side effects are domain events (`internal/domain/event.go`):
a video's ingest state changed, jobs were queued, the remaining work per stage changed, the
scan changed, and content keys lost their last reference. Publishers — `internal/store`
after a transaction commits (never from one that rolled back, and one notice per kind of
change per transaction, plus one per deleted video) and `internal/app` for job outcomes and scans — call a `Publish`
interface they declare themselves and know nothing about the subscribers. `internal/eventbus`
delivers each event to every subscriber on that subscriber's own goroutine and queue, so a
slow or panicking subscriber never stalls a commit, a worker or a scan. `cmd/mdm/events.go`
is the one place that registers subscribers (the `/api/events` stream, the worker wake-ups,
artifact removal); adding one touches only the subscriber and that file. At shutdown the
stream subscription is dropped before the streams close and the wake-ups before the workers
stop, and the bus is closed only after the workers and any running scan have stopped, so
queued artifact removals still run. The scan gets its own 10 second grace, because a read from
an unresponsive mount does not return on cancellation; past it, shutdown continues and the
next startup closes the scan.

`/api/events` pushes changes to the browser as Server-Sent Events instead of the
browser polling: `scan` when the current scan changes, `processing` with the remaining
jobs per stage, and `video` when a video's ingest state changes. The payload is read
at send time, pending notices for a connection are coalesced, and a new connection
first receives the current `scan` and `processing` so a reconnect recovers what it
missed. Logical videos are separated from their physical
locations so the same content may remain available from more than one configured root.
Folders are not stored: the folder browsing API derives each folder's direct
children and direct videos from the current locations' paths on every request,
addressing a folder by its registered root's id and a `/`-separated relative path.
Streaming delegates ranges to `http.ServeContent` and only opens current locations that
resolve inside a configured media folder.

`internal/mediafs` is the one owner of the rule that keeps arbitrary files from being
read: a location may be opened only when its cleaned path is inside a registered media
folder, the path its symbolic links resolve to is inside the same folder, and the file is
a regular file (not a directory or a device). It opens or hands out the resolved path, so
what was checked is what gets opened. Streaming, live transcoding, opening in the default
app, registering a media folder (`CheckMediaFolder`: a readable directory reached without
symbolic links) and the server-side directory picker all use it through interfaces they
declare. The containment checks themselves are pure functions in `internal/domain`
(`PathInsideRoot`, `MediaFileInsideRoot`, `SamePath`), so they are tested without a
filesystem; `internal/httpapi` only maps the result to a response, and a location that
points outside is answered like a missing file.

`internal/opener` launches the operating system's default app for a video's
representative location (`explorer.exe`, `open` or `xdg-open`). It is kept apart from
`internal/media`, which is the entry point for `ffmpeg`/`ffprobe`, and it resolves the
command once at startup; on Linux and similar systems it also requires `DISPLAY` or
`WAYLAND_DISPLAY`, so containers and headless servers report it as unavailable. The
open route only accepts requests whose remote address and `Host` are loopback, and it
never takes a path from the request.

`internal/password` hashes and verifies passwords with Argon2id and stores them as PHC
strings (`$argon2id$v=19$m=…,t=…,p=…$<salt>$<hash>`). New hashes use m=19456 KiB, t=2, p=1,
a random 16-byte salt and a 32-byte key; verification reads the parameters from the stored
string, so stronger parameters can be introduced later without invalidating existing hashes.
It is an adapter rather than part of `internal/domain` because it draws random salts and
uses an external cryptographic implementation. The pure authentication rules — the
username and password value rules, the session lifetime, the post-login redirect check,
`Audience` (whose zero value is the guest) and which list conditions a guest may use —
live in `internal/domain`.

Shutdown closes the `/api/events` streams, drains in-flight requests within a 10 second
grace period, then stops the scanner and the workers so a running job returns to the queue.

Two kinds of data live in SQLite and they are not equivalent: `videos`,
`video_locations` (including its per-location search keys), `location_search_fts`,
`jobs`, `scans`, thumbnail files, hover-preview MP4/manifest pairs, the folder index
(`folder_groups`, `folder_group_members`, `video_folder_names`, `folder_index_state`) and
the per-video live-transcode probe (`video_transcode_probes`, refilled by the next probe or
live transcode) are a rebuildable index (deleting them costs a rescan), while `playback_progress`, the tag tables
(`tags`, `tag_names`, `video_tags`), the per-video public flag (`public_videos`) and the
per-folder grouping exceptions (`folder_group_overrides`) are user data that cannot be
reconstructed.
That is why playback positions, tag assignments and public flags are keyed by the content
identifier rather than by `videos.id`, and why those tables carry no foreign
key to `videos`. Grouping exceptions are keyed by the folder's absolute path
(`domain.FolderKey`) and carry no foreign key to `videos` or `media_folders`, so they
survive rescans and media-folder changes.
The single `account` row (username, Argon2id password hash and credential version) is
also user data that cannot be reconstructed: deleting it sends the server back to first-run
setup. `sessions` belongs to neither kind; it is transient state that a fresh login
restores (`specs/016-single-account-auth/data-model.md` §2).

`store.DB` is only the foundation: it opens and closes the SQLite connection pools,
routing write transactions through an immediate-lock pool and snapshot list reads through
a deferred pool so they do not reserve the writer,
runs migrations (`store.Migrate`), answers the health ping, registers the publisher
for post-commit events, and hands out the role types. Every business operation is a
method of the role type that owns it, so calling one through the wrong role does not
compile:

- `IngestStore` — the job queue (enqueue, claim, complete, fail, requeue, remaining
  work) and writing each ingest stage's result back to the video row, including the
  retry of a failed probe and the rebuild of a missing preview. A probe's result also
  upserts the video's live-transcode probe (`video_transcode_probes`: a versioned
  `domain.TranscodeProbe` JSON plus the probed file's size and nanosecond mtime) in the
  same transaction, and `SaveTranscodeProbe` upserts one probed at request time
  (`specs/018-live-transcode-seek/data-model.md`).
- `LibraryStore` — reads of the index: the video list and search, library items
  (videos and folder groups, a group's card and the "select all" ids), folder browsing,
  related videos and the folder group a video belongs to (`VideoGroup`), a video's
  locations, a video's stored live-transcode probe (`TranscodeProbe`, read one video at a
  time and never part of the list or detail reads; `domain.TranscodeProbeUsable` decides
  whether it may be used), and the startup refresh of search keys. The
  video list can AND-filter on a set of tag ids and reports which of them do not
  exist (`VideoQuery.TagIDs`/`VideoPage.MissingTagIDs`), and `LibraryIDs` returns the
  matching item ids unpaged for the library's "select all"
  (`specs/014-video-tags/data-model.md` §6,
  `specs/017-folder-groups/contracts/library-api.md` §2). The search-box term matcher also OR-matches
  a video's tag names (original name and synonyms) alongside title and path
  (`specs/014-video-tags/data-model.md` §7). Every read that returns videos, locations
  or folders (`ListVideos`, `ListFolderVideos`, `DirectVideoPaths`, `GetVideo`,
  `VideosAddedNear`, `VideosByIDs`, `VideoGroup`, `FolderLocations`, `HasFolderLocations`) takes a
  `domain.Audience`, and its location condition goes through one function,
  `visibleLocationCondition`: the owner sees every location under a registered folder,
  a guest additionally only those of videos with a non-empty content key in
  `public_videos`, and a guest's search does not match tag names
  (`specs/016-single-account-auth/data-model.md` §3). The conditions used by ingest,
  jobs and tag counts stay owner-only. `LibraryStore` resolves which of a set of
  tag ids currently exist through `existingTagIDs`, and `TagStore` resolves a set of
  video ids down to the currently-registered videos' content keys through
  `registeredContentKeysForVideoIDs`; both are unexported package functions
  (`internal/store/roles.go`), never called as another role's public method.
- `ScanStore` — the state of a scan run.
- `ScanIndexStore` — reflecting a scan's filesystem facts into the index (upserting
  locations, removing missing ones and the videos they orphan), rebuilding the folder
  index before a scan closes, and the startup refresh of an out-of-date folder index.
- `SettingsStore` — registering, replacing and removing media folders, rebuilding the
  folder index in the same transaction.
- `FolderGroupStore` — setting and clearing a folder's grouping exception (`ungroup`,
  `group_direct`), turning a folder's group into a tag (finding or creating the tag by
  name or synonym and writing `ungroup`), each rebuilding the folder index in the same
  transaction, and reading folders' groupings for `FolderSummary.grouping`. The tag
  lookup and creation are the package-private `findOrCreateTag` and `insertTag`
  (`internal/store/tags.go`), shared with `TagStore`. The rebuild
  itself is the package-private `rebuildFolderIndex`, shared by `ScanIndexStore`,
  `SettingsStore` and `FolderGroupStore`; the assignment rule is the pure
  `domain.BuildFolderIndex` (`specs/017-folder-groups/data-model.md` §2).
- `PlaybackStore` — playback positions. It holds only the SQL connection and does not
  depend on the rebuildable index stores or their notifications.
- `TagStore` — tags themselves: create, rename, delete, merge, register/remove a
  synonym, the counted listing, and the startup refresh of tag-name search keys
  (`specs/014-video-tags/data-model.md`). It also attaches and detaches a tag across a
  set of video ids (resolved to the currently-registered videos' content keys),
  summarizes the tags on a selected set of videos, and looks up the tags on a set of
  content keys in bulk for the video list (`TagsByContentKeys`, shaped like
  `PlaybackStore.ProgressByContentKeys`). Like `PlaybackStore`, it holds only the SQL
  connection and does not depend on the rebuildable index stores or their
  notifications; tag changes have no side effects, so they publish no domain event.
- `AuthStore` — the single account and its login sessions: first-run setup (the account
  row and the first session in one transaction, so concurrent setups resolve by the
  primary key), changing the username or password (bumping `account.version` and
  clearing `sessions`), adding, checking, deleting and sweeping expired sessions. It
  stores only the SHA-256 of a session ID, and a session is valid only while its
  `account_version` matches `account.version` and it has not expired
  (`specs/016-single-account-auth/data-model.md` §4, §5). Like `PlaybackStore`, it holds
  only the SQL connection and publishes no domain event.
- `VisibilityStore` — switching the public flag of a set of video ids (resolved to the
  currently-registered videos' content keys, like tag attachment) in one transaction,
  returning the content keys it applied to
  (`specs/016-single-account-auth/data-model.md` §5). Like `TagStore`, it holds only the
  SQL connection.

`store.DB` does not hand out its `*sql.DB`, so SQL stays inside `internal/store`.
Tests outside the package set up and inspect storage through the role types, and
through the few test-only functions in `internal/store/storetest.go` (suffixed
`ForTest`, never called by production code) when no role operation fits.

A role type never calls another role type's public methods. Reads several roles
need (media folders, one video, whether a content key is still referenced) have a
single package-private implementation that each role exposes as its own operation.
Operations that span roles in one transaction — removing a media folder with its
locations, videos and jobs, or scheduling a preview rebuild — stay atomic and share
package-private SQL helpers inside that transaction.

Search matches a per-location `search_key` that Go builds from the title and the
path below the registered media folder, folded with `domain.FoldForMatch`, and
indexed by the trigram FTS5 table `location_search_fts`. SQL cannot express that
folding, so startup refreshes every location whose `search_version` is older than
`domain.SearchKeyVersion` right after `store.Migrate` and before jobs or HTTP start,
and aborts startup if that fails
(`specs/013-library-search/data-model.md` §5).

Every request crosses an authentication boundary at the outermost layer of
`internal/httpapi` (`auth.go`) before routing. It sorts each request into one of three
kinds — anyone (`GET /api/health`, `GET /api/auth/session`, `POST /api/auth/setup`,
`POST /api/auth/login`, `POST /api/auth/logout`, and `GET`/`HEAD` outside `/api/` for
the SPA build), guests too (the video, stream, artifact and folder reads), and
owner only (everything else, including undefined `/api/*` paths) — by the
`path.Clean`ed request path, so the classification matches each operation's
`security` in `api/openapi.yaml` (a Go test checks that). Because `ServeMux` splits
the escaped path before decoding each segment, a request whose escaped segments
differ from its decoded ones (an encoded `/` or `.`, as in `%2F` or `%2E%2E`) is
classified owner only when either form is under `/api/`, so classification and
dispatch cannot disagree. It decides the viewer
(`domain.Audience`) from the session cookie (`__Host-vv_session` over HTTPS,
`vv_session` over HTTP), puts it on the request context for handlers to read, and
tags every `/api/*` response with `X-VV-Audience: owner|guest`. Owner-only requests
without a valid session, and every non-anyone request while no account is configured,
get `401 unauthenticated`; a failed session lookup is `500`, never an owner.
Guest-too requests without a valid session are handled as a guest: handlers pass the
audience to `LibraryStore` and `Catalog`, so only public videos (and folders derived
from them) appear, hidden videos and folders answer the same `404` as missing ones,
guest responses omit `location`, `progress`, `probeError` and `rootPath` and carry
empty `tags`, and list conditions that depend on owner data (`watch`, played-at
sorts, `tag`) are `400`
([specs/016-single-account-auth/contracts/guest-api.md](specs/016-single-account-auth/contracts/guest-api.md)).
Thumbnails, seek previews and hover previews are served `private, no-cache` with an
`ETag` (`304` on a match) to owners and guests alike, so neither a shared cache nor
the browser keeps serving them after logout. For
owner requests the boundary keeps an in-memory ledger that sets the session expiry as
the context deadline, ends a session's in-flight responses on logout, and re-checks
requests that run longer than 30 seconds every 30 seconds so a credential change from
the host command also ends them; ending a response cancels its context and moves the
write deadline to now. `PUT /api/video-visibility` (owner only) switches the public
flag through `VisibilityStore`; after the switch commits, making videos private ends
the in-flight guest stream, live-transcode and hover-preview responses of their content
keys, which a second in-memory ledger (`visibility.go`) tracks, while owner responses
continue. Switches run one at a time from commit to cut-off, so a later re-publish
cannot be cut off by an earlier switch to private. `POST /api/auth/setup` creates the first account and logs in,
`POST /api/auth/login` and `POST /api/auth/logout` issue and revoke sessions, and
`GET /api/auth/session` reports `owner`, `guest` or `setupRequired`
([specs/016-single-account-auth/contracts/auth-api.md](specs/016-single-account-auth/contracts/auth-api.md)).
`cmd/mdm` wraps `app.Auth` for the boundary, deletes expired sessions at startup, and
logs a warning while no account is configured.
The client address and whether a request is HTTPS come from `client_origin.go`, which
reads `X-Forwarded-For` and `X-Forwarded-Proto` only on connections from the reverse
proxies in `MDM_TRUSTED_PROXIES` (loopback and private addresses when unset) and
otherwise uses the connecting address and TLS;
the login attempt limit, authentication logs, the cookie name and `Secure`, the
same-origin check and the loopback check for opening a file all use it.

Not built yet: subtitles and multi-user support. Browser-incompatible
video can be transcoded to a request-scoped fragmented MP4 stream; transcoded output is
not persisted. The transcode route reuses the video's stored live-transcode probe when it
still matches the opened file, and saves the one `internal/media` probed otherwise
([live-transcode-seek.md](docs/design-docs/live-transcode-seek.md)).

## Intended dependency direction

`cmd -> internal/{app,httpapi,store,media,mediafs,artifacts,opener,scanner,jobs,eventbus,password} -> internal/domain`, one
way only. The packages under `internal/` fall into three layers:

- `internal/domain` holds the domain model: value types and pure rules
  (`EvaluatePlayability`, `OrderRelated`, search-key folding, …), including the
  business rules the store enforces: how a claim counts attempts and whether a
  failed job returns to `queued` or stops as `failed` (`ClaimAttempts`,
  `JobStateAfterFailure`), which queued jobs may be claimed
  (`ClaimConditionFor`: a registered location, and a finished probe for
  thumbnails), and whether a media folder may be added, replaced or removed
  (`CheckMediaFolderPlacement`, `CheckMediaFolderMutation`). `internal/store`
  translates these into SQL and writes their results; it re-reads the inputs
  inside its transaction, and the database constraints (one running scan, one
  unfinished job per `(kind, video_id)`) remain the final guard against races.
  It is the end of the chain and must not depend on `net/http`, `database/sql`, `os`, `os/exec`, the
  SQLite driver, or any other `internal/*` package.
- `internal/app` is the application layer and holds the use cases: starting,
  running and closing a scan and recovering an interrupted one at startup
  (`Scans`); processing one probe, thumbnail or preview job — checking the claimed
  identity, calling the generator, applying the result, publishing the outcome, and
  removing artifacts whose content lost its last reference (`Ingest`); and the decisions behind a video response — requeueing a missing hover
  preview, deriving the seek-preview state — plus assembling related videos, which for a
  folder-group member orders next/previous inside the group and leaves its members out of
  the related list (`Catalog`); and adding, replacing and removing media folders after the
  filesystem adapter has checked the path (`MediaFolders`); and first-run setup,
  login verification with per-source throttling, and issuing, checking and
  revoking login sessions (`Auth`). It reaches storage, `ffmpeg`/`ffprobe` and generated files only
  through interfaces it declares, so its unit tests run without SQLite, `ffmpeg` or
  an HTTP server. It must not import `net/http`, `database/sql`, `os/exec`, the
  SQLite driver, or any adapter package.
- The adapters — `internal/httpapi`, `internal/store`, `internal/media`,
  `internal/artifacts`, `internal/mediafs`, `internal/opener`, `internal/scanner`, `internal/jobs` and
  `internal/password` — talk to the outside world. `internal/eventbus` sits beside them and only delivers
  `domain.Event` values in-process; only `cmd/mdm` imports it. Filesystem checks stay in the adapters: `internal/mediafs` checks
  media folder paths, the files a request may open and the directories the picker lists, so `internal/store` never touches the filesystem
  and `internal/httpapi` never decides by itself whether a file may be opened.
  `internal/httpapi` only parses requests, calls the application layer, the store or
  `internal/mediafs`, and converts to the generated `gen` types.

`cmd/mdm` is the composition root: it reads the configuration, creates the
adapters and the application-layer values, wires them together, and starts and
stops them. It holds no use case of its own.

The sibling packages under `internal/` (the adapters and `internal/app`) do not
import each other. Each declares the interfaces it consumes — `internal/app` a
scan store, an ingest store, a generator, an artifact store, an event publisher,
an auth store and a password hasher; `internal/scanner`,
`internal/jobs` and `internal/httpapi` an index to write to, a queue to claim
from, a library and a video catalog to query, generated files to serve, and an
authenticator (`httpapi.Authenticator`, which `cmd/mdm` fills by wrapping `app.Auth`) — and `cmd/mdm` is the only place
that knows which concrete type goes where. The values crossing those boundaries
(`domain.VideoFile`, `domain.Job`, `domain.VideoQuery`, `domain.VideoView`, …)
live in `internal/domain`, which is why neither side needs the other.

All of this is enforced mechanically with golangci-lint's depguard in CI. The
rules live in [.golangci.yml](.golangci.yml): one for `internal/domain`, one for
`internal/app`, and one that forbids the sibling packages from importing each
other (test files are exempt, because an external `package x_test` imports its
own package). Each rule carries the reason in its message, so a violation
explains itself from the `task lint` output alone.

The API contract in `api/openapi.yaml` is the single source of truth for the
boundary between the Go backend and the TypeScript frontend; both sides are
generated from it (`task generate`), the generated files are version
controlled, and CI fails when regenerating them produces a diff.

`web/embed.go` is the one deliberate exception to the layering: Go's embed
directive cannot reference a parent directory, so the declaration that pulls
`web/dist` into the binary lives next to the SPA and `internal/httpapi/spa.go`
consumes it as an `fs.FS`.

## Web layer

The SPA under `web/src` is split by responsibility rather than by widget.

`web/src/api/` is the only place that talks to the server. `client.ts` wraps
`fetch` over the generated types in `web/src/api/gen/` (never hand-edited;
`task generate` rewrites them from `api/openapi.yaml`), `serverEvents.ts` shares one
`EventSource` on `/api/events` among its subscribers, `useVideos.ts` owns
paging and request cancellation for the library list and re-fetches a listed video in
place when a `video` event names it. Its items are `LibraryItem`s (a video or a folder
group, `libraryItems.ts`); a group item is re-fetched from `GET /api/folders/{rootId}/group`
when a member's progress, tags or `video` event changes, and dropped on 404; a group
whose re-fetch has not settled is kept in the list snapshot's `staleGroups` and
re-fetched when the list is restored.
`useVideoDetail.ts`
fetches one video for the playback screen and re-fetches it when a `video` event names
it or the event stream reconnects, and `listSnapshot.ts`
holds the in-memory snapshot that lets the list restore its position after a
round trip to the playback screen. `tags.ts` holds a single shared, last-value-only
cache of the tag list behind `getTags`/`refreshTags`/`subscribeTags`, so the
combobox, tag-filter confirmation and the tag admin screen all read and invalidate
the same list instead of issuing their own `GET /api/tags`. `auth.ts` checks who is
viewing (`GET /api/auth/session`) and sends first-run setup, login and logout. Pages and components do
not call `fetch` themselves, so how the server is reached stays changeable in one
place.
The list's conditions (search terms, watch state, playable-only, sort and the shuffle
`seed`) live in the URL; `web/src/videoList/listCriteria.ts` converts between the URL and
the criteria `useVideos` sends, and the server applies every condition, so the page
neither filters loaded pages nor reads ahead to find matches.

`web/src/auth/` is the gate in front of every route: `AuthGate` renders nothing until
the session state is known, sends every URL to `/setup` while no account exists, sends a
guest on an owner-only screen (`/settings`, `/tags`) to `/login?next=…`, and exposes the
answer to the screens through `useAudience`. The first-run setup (`/setup`) and login
(`/login`) screens live there too and sit outside the shell and its providers. When the
viewer changes (setup, login, logout) the page is reloaded rather than re-rendered, so
nothing read for the previous viewer stays in memory. The gate also tells `client.ts` who
the page is rendered for; while that is the owner, the first `/api/*` response that is a
401 or carries `X-VV-Audience: guest` (a logout in another tab, an expired session)
reloads the page once and is never handed to the screen. The `/api/events` connection
and the playback screen's video cannot read a status or header when they fail, so when
the event stream gives up or the video fails to load they check the session instead, and
reload once if the viewer is no longer the owner rather than retrying forever. A guest gets the same shell and screens with every
owner-data control left out rather than disabled: the scan button and progress, the
tag and settings entries, selection, tag filters, the watch-state
filter and "recently played" sort (list conditions left in the URL or the stored sort are
rounded to the defaults), tags, file location, re-probe and saved playback position on the
playback screen. `ScanProvider`, `useVideos` and `useVideoDetail` neither fetch owner-only
state nor subscribe to `/api/events` for a guest.

`web/src/shell/` holds the responsive top bar, sidebar, scan state, and the
frame around a screen. `web/src/library/`, `web/src/folders/`, `web/src/settings/`,
`web/src/tags/`, and `web/src/player/` own their respective product flows, while reusable
primitives live in `web/src/ui/` and formatting helpers live in `web/src/lib/`. The
video-list pieces the library and folder screens share (list criteria and their URL hook,
the condition labels and count summary, the video card, the empty/loading/error states and
the search, filter, sort and zoom controls) live in `web/src/videoList/`, which belongs to
neither screen, so neither screen imports from the other. `web/src/tags/` is the tag
admin screen (`/tags`): a list of every tag with its video count, an in-page name/synonym
search, create, rename and delete. `web/src/shell/navigation.ts` puts its sidebar entry
right after "フォルダ" (Folders). Every sidebar entry links to a working screen. The library, folder, settings and tag screens use the shell: `app/App.tsx`
puts `AppShell` around the `/`, `/folders/*`, `/settings` and `/tags` routes, and the
playback screen
(`/videos/:id`) deliberately gets no shell at all, because it is a
two-pane screen of its own under its own header band (a logo that goes home, a
breadcrumb to the video's folder, and a single close button — × or Esc — that
returns to the list the screen was opened from): the player with the title, tags,
a file-facts row and a technical row on the left, related videos on the right. Keeping that choice to
the one routing decision is what lets the shell stay ignorant of which screen it
is framing. Inside `web/src/player/`, video.js owns only the control bar; ingest
stages, read and playback failures, the ended prompt (for a folder-group member with
a next member, a five-second autoplay notice that re-checks that member before moving on)
and the touch controls are React layers stacked in one container above the player, and keyboard shortcuts are
handled page-wide rather than by video.js. The composition is recorded in
[docs/design-docs/library-ui.md](docs/design-docs/library-ui.md).
The shell exposes the library, the folder browser and media-folder settings as routes.
The folder browser reuses the shared video card and the library's paging (`useVideos` takes the
folder as its source) and reads its location from the URL itself: each path segment is
encoded once when a link is built and decoded once from `location.pathname`, so names
containing `%`, `#` or `?` round-trip. "Recently added"
and "In progress" show a preparation notice until backing routes exist; shell tests
keep that boundary explicit.

The shell does not take ownership of scrolling. The sidebar and the toolbar are
fixed or sticky, and the document (the window) keeps scrolling the content as
it did before the shell existed. That is deliberate: the library list's scroll
restoration, its zoom anchoring and its infinite scroll all sit on
`window.scrollY` and on a viewport-based `IntersectionObserver`, so moving the
scroll container inside the shell would rewrite all three.

`web/src/index.css` is the single source of truth for the visual rules. Its
`@theme` block declares every color, radius and size as a role-named token
(`--color-surface`, `--color-fg-muted`, `--color-accent`, …), and
screens use only the utility classes generated from it. Raw hex values, raw
pixels and Tailwind's default palette names are not written under `web/src/**`.
Only the dark palette is implemented; there is no light/dark toggle. The
reasoning is recorded in
[docs/design-docs/library-ui.md](docs/design-docs/library-ui.md).

`web/src/theme/` contains no runtime code — it is inspection only.
`tokens.test.ts` reads `index.css` as a file and asserts that every token pair
it lists meets its WCAG contrast ratio, and that no file under `web/src`
reintroduces a raw color or a Tailwind palette name.
`web/src/preferences/` holds the per-device display settings as total
functions over
`localStorage` that never throw, so a corrupted value degrades to the defaults
instead of blanking the screen.

Unit tests run on Vitest with Testing Library in a `jsdom` environment,
configured in `web/vite.config.ts` and `web/vitest.setup.ts`. `task test-web`
runs the production build check and `vitest run` together, and `task check`
calls it, so a regression in either fails CI the same way.

## Principles

- Make module boundaries explicit and mechanically enforceable where possible.
- Keep dependencies directed from product-facing layers toward stable domain
  interfaces.
- Capture consequential design decisions in `docs/design-docs/`.
- Keep generated code (`internal/httpapi/gen/`, `web/src/api/gen/`) generated;
  change `api/openapi.yaml` instead.
