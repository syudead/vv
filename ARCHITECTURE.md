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
metadata, thumbnails, previews and live transcoding, driven by an in-process job
worker or by the request that needs them. Sidecar subtitles are converted to WebVTT
in Go, without `ffmpeg`. Everything ships as one container.

In place today: `cmd/mdm` reads the remaining `MDM_*` environment variables, checks that
`ffprobe`/`ffmpeg` are on `PATH`, opens SQLite under `MDM_DATA_DIR` and applies
embedded goose migrations at startup, then starts the job worker. It serves `GET /api/health`,
the video library API (`/api/videos*`, `/api/scans*`; a single video's response also
carries its representative location, the folder that holds it (with the registered folder's
display name, for the playback page's breadcrumb), seek-preview state and, for a folder-group
member, the group and its position in it, and, for a member of a bundle of versions, the
number of versions the viewer may see and the effective representative (`versions`), and
`/api/videos/{id}/related`, `/probe` and `/open` return related videos (for a group member,
also every member in group order, with next/previous inside the group), retry a failed
metadata read, and open the file in the server PC's default app), media-folder settings and
server-side directory picker APIs, the live-transcode video encoder setting
(`/api/settings/transcoding`: the saved choice, the encoder in use and each hardware encoder's
startup check result), the read-only folder browsing API
(`/api/folders*`), the tag management API (`/api/tags*`: list, create,
rename, delete, merge and synonym registration/removal), the video-tags API
(`/api/video-tags` to attach/detach a tag on a set of videos and
`/api/video-tags/summary` to summarize which tags apply to a selection),
the library items API (`/api/library*`, below), the versions API (below), byte-range streaming,
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

Every API error is a JSON `Error` with a machine-readable `code` and an English
`message`. Where one `code` covers several situations the UI can cause, the
response also carries a `reason` (`ErrorReason`), and for some reasons a server
`limit` or the conflicting tag's original name (`tagName`); the UI builds its text
from `code` and `reason`, and the `message` is the fallback for API clients and
unknown codes. `internal/httpapi` fills `limit` from the server's own constants and
maps `domain.InvalidTagNameError` to the tag-name reasons
([specs/023-english-i18n/contracts/error-api.md](specs/023-english-i18n/contracts/error-api.md)).

`GET /api/library` takes the same parameters as `GET /api/videos` but returns
library items: a video, or a folder group as one item
(`LibraryStore.ListLibrary`). The search and tag filters apply per member. A
group is one item only when all of its members match; when some but not all
match, each matching member is listed as its own video item. With no search or
tag filter every group matches in full. A group's values (count, total
duration and size, latest dates, watch state and the member to open, decided by
`domain.GroupWatch` and `domain.GroupOpenIndex`) come from all of its members,
and the playable filter, watch filter, sort and `total` apply to items (a group
is playable when any member is). `GET /api/library/ids`
returns the ids of the listed video items plus every member of listed groups
(owner only), and `GET /api/folders/{rootId}/group` refetches one group card. A guest sees
groups built from public members only; a group with one public member is listed
as that video, and "all members" counts public members only
([specs/017-folder-groups/contracts/library-api.md](specs/017-folder-groups/contracts/library-api.md),
[specs/027-partial-group-search/contracts/library-api.md](specs/027-partial-group-search/contracts/library-api.md)).
`GET /api/videos` stays a per-video list for the folder view's root search.

Every list folds a bundle of versions of the same video to its effective
representative: the representative when the viewer may see one of its
locations, otherwise the lowest-id version the viewer may see; with none, the
bundle is not listed. `chosenLocationsCTE` applies the scope (library, public,
folder) to the representative's locations only and matches the search
expression against the registered locations of any version in the bundle, and
the folder view and folder counts, the related list, the tag counts and the
folder-index input (`folderIndexLocations`) use the same rule
(`shownVideoCondition` in `internal/store/user_keys.go`). A folder holding only
non-representative versions is a folder with no videos, and those versions join
no folder group. `VersionStore` rebuilds the folder index in the transaction
that bundles, changes the representative or unbundles. `GET /api/videos/{id}`,
locations, streaming and subtitles still serve every version
([specs/030-video-versions/data-model.md](specs/030-video-versions/data-model.md) §4).

The versions API addresses a bundle by any of its videos' ids and never exposes the
bundle's own id. `GET /api/videos/{id}/versions` (owner and guest) returns the versions the
viewer may see, the effective representative first and the rest in natural title order,
each shaped like `GET /api/videos/{id}`; a video outside any bundle returns itself alone.
The owner-only `POST /api/video-bundles` (`videoIds`, `representativeId`, the same
20,000-id cap as `/api/video-tags`), `POST /api/videos/{id}/make-representative` and
`POST /api/videos/{id}/unbundle` call `VersionStore` directly and map its errors to the
`too_few_videos`, `representative_not_selected` and `not_bundled` reasons
([specs/030-video-versions/contracts/screen-api.md](specs/030-video-versions/contracts/screen-api.md) §0–§4).
The owner-only `GET /api/version-candidates` lists up to 200 "possibly the same video"
pairs, newest first, each video shaped like `GET /api/videos/{id}`, and
`POST /api/version-candidates/dismiss` records a pair as different videos
(`internal/httpapi/version_candidates.go`, §5). "Same video" goes through
`POST /api/video-bundles`. Candidates change with fingerprint jobs, whose
`ProcessingChanged` already reaches the screen as the `scan` event, so no new event kind
is added (§6).

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
When a path's content changes and the previous video row goes with it, `UpsertVideo` records
the previous content key and duration in `video_successions` (not when that duration is
unknown), and drops the record if the previous content shows up at another path. The record
is judged only after a scan closes `done` — in `FinishScan`'s transaction, or in the
transaction that writes the new content's probe result if that comes later — so a file swap
is recognised whatever order the paths are walked in. When the durations match
(`domain.DurationsMatch`) and no video still has the previous content, the new content takes
over its tags, playback position, public flag and "different video" judgements, or its place
in a version bundle, and `domain.VideoBundleChanged` is published after commit
([specs/030-video-versions/data-model.md](specs/030-video-versions/data-model.md) §5).
The latest scan owns the set of videos that the current import has to prepare
(`scan_videos`): every transaction that queues a job, or makes a queued job claimable again
by adding or replacing a media folder, adds the video to the latest scan in the same
transaction, and starting a scan replaces the previous set while carrying over the videos
that still have claimable queued or running jobs. The probe result and the preview job are
written in one transaction, so a video never looks finished between the two. A video is
settled when it has no claimable queued or running job, derived on every read, so requeued
running jobs are not counted twice. `scans.settled_at` records when the closed latest scan's
set first had no remaining work; `internal/store` recomputes it (`refreshScanSettled`) before
committing any transaction whose change set can alter the remaining work — claims, job
outcomes, enqueues, video deletions, media-folder changes and closing a scan.
`internal/app` (`Scans`) combines the scan row and those counts into the user-facing
`Scan.status` (`finding`, `running`, `done`, `partial`, `failed`) and progress through
`domain.ImportTally`
([specs/024-import-progress/research.md](specs/024-import-progress/research.md) R-1–R-5).
The latest scan also owns its issues (`scan_issues`, one row per event): `internal/scanner`
hands each file it cannot read or register to the reporter it declares, and `internal/app`
records it; a job that fails at the retry limit is recorded in the same transaction as
`recordTerminalFailure`, and a later success of that stage deletes it. Starting a scan clears
the previous scan's issues, and every change bumps `scans.issues_revision`. Reads group the
rows per video (or per path for unregistered files) and drop those outside every registered
media folder; one failed issue makes the import `partial`, and files that could not be
registered count toward the progress (R-6).
What the import is doing right now (`Scan.activity`: registering, probe, thumbnail,
seekThumbnail or preview, with the file) is not stored: `internal/app` (`Scans`) keeps the
running activities in memory, shows the one that started last and falls back to the latest
remaining one when it ends, and publishes `ScanActivityChanged` whenever the shown one
changes (R-8). `internal/scanner` reports each file before registering it and its progress
after every file (no longer every 20 files) to the reporter it declares, and the workers
report each job through the `Started` and `Finished` hooks of `internal/jobs`, which
`cmd/mdm` wires to `Scans`. After a restart only what is running then is shown.
`internal/jobs` runs one in-process worker per ingest stage — probe, thumbnail,
seek_thumbnail, preview, fingerprint — each claiming only its own kind of job from the persistent `jobs`
queue, one at a time, and handing it to `internal/app`, which drives the `internal/media`
adapters (`ffprobe` for metadata, `ffmpeg` for one library thumbnail, seek-preview sprite
sheets from the keyframes an MP4/MOV index assigns to each interval or, for other inputs,
from up to four concurrent input seeks,
and a content-keyed hover-preview clip per video)
and publishes their output through `internal/artifacts`. New seek previews use at most 81
frames on one 9 × 9 sheet with cells up to 160 px. `domain.NewSeekSpriteLayout` derives
the interval from the duration, keeping five seconds for short videos and widening it
for longer videos. For MP4/MOV with H.264/HEVC the index is read once and only the chosen
keyframes are read; other inputs use one input seek per frame. An interval without a frame
reuses the previous frame, and only an ffmpeg failure falls back to the sequential decoder.
Existing completed six-sheet sprites remain readable
([seek-sprite-generation.md](docs/design-docs/seek-sprite-generation.md)).
The fingerprint stage turns a completed seek sprite into the video's visual fingerprint
without starting `ffmpeg` or reading the source file again: `internal/app` reads the sprite's
layout and sheets through its `ArtifactStore`, `internal/media` (`SpriteFingerprint`) cuts
each frame, drops dark edge rows and columns and shrinks it to 32 × 32 luma, and
`internal/domain` hashes each frame with a DCT-based pHash (`HashFrame`) and compares two
fingerprints by pairing frames by time, not by index (`CompareFingerprints`), because the
sprite interval differs between encodes of a video longer than 405 seconds. The result is
stored per content key in `video_fingerprints` together with `domain.FingerprintVersion`
([specs/030-video-versions/research.md](specs/030-video-versions/research.md) R-6).
The transaction that stores a fingerprint also rebuilds that content's rows in
`video_version_candidates`: other contents with a fingerprint of the same version whose
duration is within `DurationsMatch` and whose distance, computed by the deterministic SQLite
function `vv_fingerprint_distance` (registered like `vv_shuffle_key` and calling
`CompareFingerprints`), is at most `FingerprintMatchMaxDistance`. Pairs recorded as different
(`video_version_dismissals`) and pairs in the same bundle are left out; candidates are never
bundled automatically (R-7).
Two generation fallbacks are substitutions that the user is told about: a library thumbnail
taken from the first frame because no frame was found at the chosen position, and a seek
sprite rebuilt by decoding the whole video. `internal/media` returns them as values
(`Thumbnail`, `GenerateSeekSprite`) without knowing events or the store, `internal/app`
passes them with the stage's success, and `internal/store` records
`thumbnail_first_frame` / `seek_thumbnail_full_decode` in the transaction that writes the
success, or deletes the row when the stage is rebuilt without the fallback. The seek sprite
also keeps the flag in its `sprite.json`, so adopting a completed sprite (a rerun after a stop
between publishing and recording, or another video with the same content) records the same
substitution. An import with
only substitutions stays `done` and counts them in `Scan.issues.substituted`
([specs/024-import-progress/research.md](specs/024-import-progress/research.md) R-7).
The library thumbnail and the seek
sprite are separate stages with their own state columns, so a library thumbnail never waits
for any video's seek sprite. A worker sleeps while its queue is
empty: `internal/store` publishes `domain.JobsQueued` after every committed enqueue, and a
subscription wakes the worker for that stage, so no worker polls the queue. A thumbnail job
is not claimed until its video's probe has finished, because the frame position depends on
the duration. A seek_thumbnail job is not claimed until its video's probe has finished and
no claimable thumbnail job remains, so after a scan every library thumbnail comes first and
up to four generation `ffmpeg` processes can run within one seek job. Hover preview and
newly claimed thumbnail work may overlap; the condition applies only at claim time, and a
running seek_thumbnail job is not
stopped when new thumbnail jobs arrive. A fingerprint job is not claimed until its video's seek
sprite is `done`; the transaction that records a finished seek sprite (including one rebuilt
after `RequeueMissingSeekThumbnails`) queues it, so its worker needs no other wake-up.
Rebuilding a missing sprite drops the waiting fingerprint job, and the next scan requeues a
fingerprint that failed at the retry limit or was made by an older version
(`IndexedVideo.FingerprintMissing` and `EnsureJob`). `internal/app` publishes `domain.VideoIngestChanged`
with the finished stage, and subscriptions wake the thumbnail worker as soon as a probe's
result is recorded, and the seek_thumbnail worker when a probe or thumbnail result is
recorded or a video row is deleted. Removing a media folder also publishes
`domain.JobsQueued` for seek_thumbnail, because thumbnail work under that folder stops
being claimable even when the video row survives through an unregistered location.
Interrupted scans are closed,
running jobs are requeued, and the single `.tmp` directory that holds in-progress
generation output is removed at the next startup. When a video row is deleted (a scan finds its last
location gone, its content changes, or its media folder is removed or replaced),
`internal/store` publishes the released content keys (`domain.ContentUnreferenced`) after
commit, and `internal/app`, subscribed to that event, removes that content's thumbnail, seek sprite and hover preview unless another video still
references it; nothing else sweeps the thumbnails directory. A hover preview that is gone
or incomplete is repaired when it is found: `internal/app` already checks it before the
video API exposes `previewUrl`, and when a `done` preview's MP4 is missing or does not match
the size in its manifest it sets the video back to `pending` and queues a preview job in one
transaction, once per loss.

A failed probe or scan keeps its free-text reason (`videos.probe_error`, `scans.error`) and,
separately, a machine-readable code: `videos.probe_error_code`, and `scans.error_code` with
`scans.error_path` when the reason concerns one location (the media folder or the place that
could not be read). The adapter that produces the failure wraps it in `domain.ProbeFailure`
(`internal/media`) or `domain.ScanFailure` (`internal/scanner`); `internal/app` marks a scan
stopped by shutdown as `interrupted`, as does closing one left running at startup; and
`internal/store` takes the code out with `errors.As` when it records the failure, storing
`internal` for anything unwrapped. The API returns `Video.probeErrorCode` and
`Scan.errorCode`/`errorPath`, so the screen explains a failure from the code instead of the
free text. Rows recorded before the codes existed keep their old reason with no code
([data-model.md](specs/023-english-i18n/data-model.md)). `jobs.last_error` stays free text.

Generated files have one owner, `internal/artifacts`. Under `MDM_DATA_DIR/thumbnails`
(the root comes from `cmd/mdm`'s configuration) it alone decides where each content key's
files live — the library thumbnail at `<p>/<s>.jpg`, the seek sprite under `seek/<p>/<s>/`
(sheets `000.jpg`… and the `sprite.json` layout, whose presence marks the sprite complete),
the hover preview and its size/SHA-256 manifest at `preview/<p>/<s>.mp4[.sha256]`, where
`<s>` is the content key with `:`, `/` and `\` replaced by `_` and `<p>` its first two
characters — and it alone creates, checks, opens and removes them. Generation writes into
a directory under `.tmp` that `internal/artifacts` hands out and then renames into place,
so a file still being generated is neither reported as present nor served. The seek
sprite's layout records the frame size read from the first sheet, and publishing it first
removes a directory without `sprite.json` (the earlier one-JPEG-per-frame layout or a broken
one) under the caller's per-content lock. A content key
that would point outside the root (empty, or starting with `.`) never becomes a path.
`internal/media` only runs `ffmpeg` against the output path it is given; `internal/app`
(deciding when to generate and when to remove, under its per-content lock) and
`internal/httpapi` (serving the files) reach the store through interfaces they declare.
Changing that layout would orphan every file an existing data directory already holds.

State changes that trigger side effects are domain events (`internal/domain/event.go`):
a video's ingest state changed, jobs were queued, the remaining work per stage changed, the
scan changed, the current import activity changed (`ScanActivityChanged`), a video's
owner override changed (`VideoOverrideChanged`), and content keys lost their last reference. Publishers — `internal/store`
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
browser polling: `scan` when the current scan, the remaining jobs (job outcomes
move the import's settled count) or the current activity change, and `video` when a
video's ingest state or its owner override (display name) changes. There is no per-stage job count on the wire; the screen
shows only the import's own status, video count and current activity. The payload is read
at send time, pending notices for a connection are coalesced, and a new connection
first receives the current `scan` so a reconnect recovers what it missed. Logical videos are separated from their physical
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
what was checked is what gets opened. Streaming, live transcoding, sidecar subtitles,
opening in the default app, registering a media folder (`CheckMediaFolder`: a readable directory reached without
symbolic links) and the server-side directory picker all use it through interfaces they
declare. The containment checks themselves are pure functions in `internal/domain`
(`PathInsideRoot`, `MediaFileInsideRoot`, `SamePath`), so they are tested without a
filesystem; `internal/httpapi` only maps the result to a response, and a location that
points outside is answered like a missing file. For subtitles it lists the regular files
next to a location that passes the same rule (`ListSidecarFiles`) and opens only a name
from that listing (`OpenSidecarFile`), so a request never builds a path.

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

### Rebuildable and user data

Stored data falls into three recovery categories. `videos`, `video_locations`
(including their search keys), `location_search_fts`, `jobs`, `scans` (including
`settled_at` and `issues_revision`), `scan_videos`, `scan_issues`, generated
thumbnails and previews, the folder index (`folder_groups`, `folder_group_members`,
`video_folder_names`, `folder_index_state`), `video_transcode_probes`, the pending
same-path successions (`video_successions`), the visual fingerprints (`video_fingerprints`), and the
version candidates (`video_version_candidates`) are
rebuildable from registered media folders by scanning and processing the files again.
`playback_progress`, the tag tables (`tags` including its `tentative` flag, `tag_names`,
`video_tags`, and `rejected_tag_names`, `specs/031-tentative-tags/data-model.md` §1),
`public_videos`, `video_overrides` (owner-set display names and representative thumbnail
positions, `specs/029-video-overrides/data-model.md` §1), the version bundles
(`video_bundles`, `video_bundle_members`) and the "different video" judgements
(`video_version_dismissals`, `specs/030-video-versions/data-model.md` §1), `folder_group_overrides`,
`account`, `media_folders`, `settings` (owner-chosen values such as the live-transcode video encoder,
`specs/025-hardware-encoding/data-model.md`), and `api_tokens` (issued API tokens, which
must be issued again if lost, `specs/026-external-api/data-model.md` §1) are user or
configuration data that a scan cannot restore. In particular, a scan
cannot start with no registered `media_folders`; after database loss those folders
must be registered again before scanning. `sessions` is transient and a fresh
login restores it.
That is why playback positions, tag assignments, public flags and video overrides are keyed by the content
identifier rather than by `videos.id`, and why those tables carry no foreign
key to `videos`. For a video that belongs to a bundle of versions, playback positions,
tag assignments and public flags are keyed by the bundle's own `user_key`
(`bundle:<id>`) instead; the one expression `userKeyExpr`
(`internal/store/user_keys.go`) picks the key for every read and write, and every read that
returns a video carries it as `Video.UserKey` (`specs/030-video-versions/data-model.md` §3).
Video overrides and generated files stay keyed by the content identifier. Grouping exceptions are keyed by the folder's absolute path
(`domain.FolderKey`) and carry no foreign key to `videos` or `media_folders`, so they
survive rescans and media-folder changes.
The single `account` row holds the username, Argon2id password hash and credential
version; deleting it sends the server back to first-run setup
(`specs/016-single-account-auth/data-model.md` §2). The operational backup and
restore procedure is in [Running vv](docs/how-to/running-vv.md#data-and-recovery).

`store.DB` is only the foundation: it opens and closes the SQLite connection pools,
routing write transactions through an immediate-lock pool and snapshot list reads through
a deferred pool so they do not reserve the writer,
runs migrations (`store.Migrate`), answers the health ping, registers the publisher
for post-commit events, and hands out the role types. Every business operation is a
method of the role type that owns it, so calling one through the wrong role does not
compile:

- `IngestStore` — the job queue (enqueue, claim, complete, fail, requeue, remaining
  work) and writing each ingest stage's result back to the video row, including the
  retry of a failed probe and the rebuild of a missing preview, and replacing a content's
  fingerprint and its version candidates (`ApplyFingerprintForJob`). A probe's result also
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
  video ids down to the currently-registered videos' user keys through
  `userKeysForVideoIDs` (`OverrideStore` uses the content-key variant
  `registeredContentKeysForVideoIDs`); these are unexported package functions
  (`internal/store/roles.go`), never called as another role's public method.
- `ScanStore` — the state of a scan run.
- `ScanIndexStore` — reflecting a scan's filesystem facts into the index (upserting
  locations, removing missing ones and the videos they orphan), rebuilding the folder
  index before a scan closes, and the startup refresh of an out-of-date folder index.
- `SettingsStore` — registering, replacing and removing media folders, rebuilding the
  folder index in the same transaction; and reading and saving owner settings in the
  `settings` table (the live-transcode video encoder choice, returned uninterpreted).
- `FolderGroupStore` — setting and clearing a folder's grouping exception (`ungroup`,
  `group_direct`), turning a folder's group into a tag (finding or creating the tag by
  name or synonym and writing `ungroup`), each rebuilding the folder index in the same
  transaction, and reading folders' groupings for `FolderSummary.grouping`. The tag
  lookup and creation are the package-private `findOrCreateTag` and `insertTag`
  (`internal/store/tags.go`), shared with `TagStore`. The rebuild
  itself is the package-private `rebuildFolderIndex`, shared by `ScanIndexStore`,
  `SettingsStore`, `FolderGroupStore` and `VersionStore`; the assignment rule is the pure
  `domain.BuildFolderIndex` (`specs/017-folder-groups/data-model.md` §2).
- `PlaybackStore` — playback positions. It holds only the SQL connection and does not
  depend on the rebuildable index stores or their notifications.
- `TagStore` — tags themselves: create, rename, delete, merge, register/remove a
  synonym, the counted listing, and the startup refresh of tag-name search keys
  (`specs/014-video-tags/data-model.md`). It also attaches and detaches a tag across a
  set of video ids (resolved to the currently-registered videos' user keys, one per
  bundle, through `userKeysForVideoIDs`),
  summarizes the tags on a selected set of videos, and looks up the tags on a set of
  user keys in bulk for the video list (`TagsByContentKeys`, shaped like
  `PlaybackStore.ProgressByContentKeys`). The external API's bulk by-name operation can
  create new tags as tentative, skipping names the owner rejected; the store confirms a
  tentative tag, rejects it (deleting it and remembering its name in
  `rejected_tag_names`), lists and forgets rejected names, and every entry that writes a
  name into `tag_names` removes it from the rejected names in the same transaction
  (`specs/031-tentative-tags/data-model.md`). Like `PlaybackStore`, it holds only the SQL
  connection and does not depend on the rebuildable index stores or their
  notifications; tag changes have no side effects, so they publish no domain event.
- `AuthStore` — the single account, its login sessions and its API tokens: first-run
  setup (the account row and the first session in one transaction, so concurrent setups
  resolve by the primary key), changing the username or password (bumping
  `account.version` and clearing `sessions` and `api_tokens` in the same transaction),
  adding, checking, deleting and sweeping expired sessions, and issuing, listing, revoking
  and checking API tokens and recording their last use at most once a minute. It stores
  only the SHA-256 of a session ID or an API token; a session is valid only while its
  `account_version` matches `account.version` and it has not expired
  (`specs/016-single-account-auth/data-model.md` §4, §5), and an API token only while its
  `account_version` matches (`specs/026-external-api/data-model.md` §1). Like
  `PlaybackStore`, it holds only the SQL connection and publishes no domain event.
- `VisibilityStore` — switching the public flag of a set of video ids (resolved to the
  currently-registered videos' user keys, like tag attachment) in one transaction,
  returning the content keys it applied to (every member's content key for a bundle)
  (`specs/016-single-account-auth/data-model.md` §5). Like `TagStore`, it holds only the
  SQL connection.
- `OverrideStore` — an owner's display name for a video (resolved to its content key,
  like tag attachment), one video or an external-API batch in one transaction, together
  with rewriting the `title_key` and `search_key` of every location of that content
  (`specs/029-video-overrides/data-model.md` §3, §4). It uses the SQL connection and
  publishes `domain.VideoOverrideChanged` after the commit. Every read that returns a video
  carries the override: `Video.Title` is the display name when set, `Video.FileTitle` the
  location's file-derived title. Releasing generated files (`RemoveContent`) never
  touches `video_overrides`. `PUT /api/videos/{id}/display-name` (owner only,
  `internal/httpapi/video_overrides.go`) calls it directly and answers with the same
  `Video` as `GET /api/videos/{id}`; guest responses carry the display name in `title`
  and omit `fileTitle`, `displayName` and `thumbnailPositionMs`
  (`specs/029-video-overrides/contracts/screen-api.md`).
  `PUT /api/videos/{id}/thumbnail-position` instead goes through
  `app.Ingest.SetThumbnailPosition` (the `httpapi.ThumbnailPicker` interface), which
  regenerates the thumbnail under the same generation lock as the ingest job before
  recording the position; the handler resolves the source file with the same rule as
  `getVideoStream` and answers once generation has finished. Inside the lock it
  rechecks, before generating, after generating but before publishing, and again
  before recording, that the chosen location still belongs to the video's content
  (a scan does not take that lock), and when recording fails after publishing it
  restores the previous image
  (`artifacts.Store.StashThumbnail`), so the image always matches the recorded
  position and revision.
- `VersionStore` — bundling videos as versions of the same video, changing a bundle's
  representative, removing a video from its bundle, and reading a bundle's versions
  (`specs/030-video-versions/data-model.md` §8). Bundling copies the representative's
  user-keyed values to the new bundle's `user_key` and leaves each member's content-keyed
  rows untouched, so a removed version returns to its own values; dissolving a bundle down
  to one video copies the bundle's values onto that video's content key. Each operation is
  one transaction that rebuilds the folder index and publishes `domain.VideoBundleChanged`
  after the commit, which the screen subscription turns into a `video` notification per
  affected video.
  It also lists the version candidates whose two contents both have a registered
  location (`Candidates`) and records a pair as different videos (`Dismiss`), which removes
  the pair's candidate. Bundling, and a same-path succession that moves bundle members or
  dismissals to a new key, drop candidates that became same-bundle or dismissed pairs.

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
`internal/httpapi` (`auth.go`) before routing. It sorts each request into one of four
kinds — Bearer (`/api/v1/…`, the external API, and `/mcp`; see below), anyone (`GET /api/health`, `GET /api/auth/session`, `POST /api/auth/setup`,
`POST /api/auth/login`, `POST /api/auth/logout`, and `GET`/`HEAD` outside `/api/` for
the SPA build), guests too (the video, stream, subtitle, artifact and folder reads), and
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
guest responses omit `location`, `progress`, `probeError`, `probeErrorCode` and `rootPath` and carry
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
`GET`/`POST /api/api-tokens` and `DELETE /api/api-tokens/{id}` (owner only, cookie
sessions only) list, issue and revoke API tokens; the plaintext appears only in the
issue response
([specs/026-external-api/contracts/token-api.md](specs/026-external-api/contracts/token-api.md)).
Bearer requests — a `path.Clean`ed path under `/api/v1/` (including undefined ones,
which answer a JSON `404` after authentication) or `/mcp`, chosen only when the
escaped and decoded segments agree — never read the cookie: `Authorization: Bearer
<token>` alone decides the owner through `app.Auth`, and a missing, malformed, unknown
or revoked token is `401 unauthenticated` with `WWW-Authenticate: Bearer`. Every
other path never reads `Authorization`, so a Bearer-only request to the screen API is
a guest (or `401` for owner-only operations). The same-origin check applies to cookie
requests only; a Bearer request's `Origin` is not checked, while the JSON body rule
still applies. A valid token records its last use at most once a minute (a failed
write is logged and the request continues), and its requests join the in-memory
ledger under the token's id, so revoking it on the settings page ends them at once and
the 30-second re-check ends them after a credential change from the host command
([specs/026-external-api/research.md](specs/026-external-api/research.md) R-3, R-4,
R-9). The external API's handlers live on a separate type (`externalServer`,
`external.go`) generated from `api/external-v1.yaml` into `internal/httpapi/extgen/`
([docs/how-to/external-api.md](docs/how-to/external-api.md)).
`/mcp` is an MCP server (Streamable HTTP, stateless, JSON responses; `GET` and `DELETE`
are `405`) built with the official Go SDK in `internal/httpapi/mcp.go`, behind the same
Bearer boundary. Its eight tools map one to one to the external API operations: each tool
builds that operation's query or body, calls the same `externalServer` handler, and
returns the response body as structured content, or `isError` with the external API's
error body for a non-2xx status. A tool call is cancelled with its HTTP request, so
revoking a token also ends its running tools
([specs/026-external-api/contracts/mcp.md](specs/026-external-api/contracts/mcp.md),
[research.md](specs/026-external-api/research.md) R-8).
`cmd/mdm` wraps `app.Auth` for the boundary, deletes expired sessions at startup, and
logs a warning while no account is configured.
The client address and whether a request is HTTPS come from `client_origin.go`, which
reads `X-Forwarded-For` and `X-Forwarded-Proto` only on connections from the reverse
proxies in `MDM_TRUSTED_PROXIES` (loopback and private addresses when unset) and
otherwise uses the connecting address and TLS;
the login attempt limit, authentication logs, the cookie name and `Secure`, the
same-origin check and the loopback check for opening a file all use it.

Sidecar subtitles are found on every request: `GET /api/videos/{id}/subtitles` reads the
folder of the first location streaming can open, and `domain.SubtitleSidecars` turns its
entries into `<name>.srt`/`.vtt` and `<name>.<label>.srt`/`.vtt` tracks (no SQLite
change, `no-store`). `GET /api/videos/{id}/subtitles/{file}` reopens only a listed name,
and `internal/media`'s `SubtitleConverter` detects the character encoding, turns SRT into
WebVTT and shifts cues by `offsetMs`; it is served `private, no-cache` with an `ETag`.
Both are guest-too routes behind `lookupServedVideo`
([sidecar-subtitles.md](docs/design-docs/sidecar-subtitles.md)).

Not built yet: multi-user support. Browser-incompatible
video can be transcoded to a request-scoped fragmented MP4 stream; transcoded output is
not persisted. The transcode route reuses the video's stored live-transcode probe when it
still matches the opened file, and saves the one `internal/media` probed otherwise.
A seek copies the video when it can and starts at the previous keyframe; `internal/media`
reads that actual start from the output's `moov` edit lists and falls back to encoding
when it is more than `domain.CopySeekAllowance` before the requested position.
The player learns that start from `GET /api/videos/{id}/transcode-start`, keyed by the
`attempt` it put on the transcode URL and served from an in-memory ledger in `internal/httpapi`
([live-transcode-seek.md](docs/design-docs/live-transcode-seek.md)).
When the video is encoded, the route puts the encoder `TranscodeSettings` currently resolves
(software, NVENC, Quick Sync, VAAPI or VideoToolbox) on each request, so a new choice applies
from the next request without a restart and a running stream keeps the encoder it started
with. A hardware encoder that exits before its first data is retried with `libx264` inside
the same request, and the route logs that fallback as a warning. `cmd/mdm` starts the
startup encoder checks in the background, so they never delay the HTTP listener
([hardware-encoding.md](docs/design-docs/hardware-encoding.md)).
An optional `quality` (`1080p`, `720p`, `480p`, `360p`) on the transcode URL asks for a
smaller picture: the route accepts only a quality below the video's display short side
(`domain.TranscodeQuality.Available`) and answers 400 otherwise, and `internal/media` then
always encodes from the exact `startMs`, scales the short side down and caps the bitrate.
The server keeps no quality state; each request carries its own
([playback-quality.md](docs/design-docs/playback-quality.md)).

## Intended dependency direction

`cmd -> internal/{app,httpapi,store,media,mediafs,artifacts,opener,scanner,jobs,eventbus,password} -> internal/domain`, one
way only. The packages under `internal/` fall into three layers:

- `internal/domain` holds the domain model: value types and pure rules
  (`EvaluatePlayability`, `OrderRelated`, search-key folding, …), including the
  business rules the store enforces: how a claim counts attempts and whether a
  failed job returns to `queued` or stops as `failed` (`ClaimAttempts`,
  `JobStateAfterFailure`), which queued jobs may be claimed
  (`ClaimConditionFor`: a registered location, a finished probe for
  thumbnails and seek thumbnails, no claimable thumbnail job left for seek
  thumbnails, and a finished seek sprite for fingerprints), and whether a media folder may be added, replaced or removed
  (`CheckMediaFolderPlacement`, `CheckMediaFolderMutation`). `internal/store`
  translates these into SQL and writes their results; it re-reads the inputs
  inside its transaction, and the database constraints (one running scan, one
  unfinished job per `(kind, video_id)`) remain the final guard against races.
  It is the end of the chain and must not depend on `net/http`, `database/sql`, `os`, `os/exec`, the
  SQLite driver, or any other `internal/*` package.
- `internal/app` is the application layer and holds the use cases: starting,
  running and closing a scan and recovering an interrupted one at startup
  (`Scans`); processing one probe, thumbnail, seek-thumbnail, preview or fingerprint job — checking the claimed
  identity, calling the generator, applying the result, publishing the outcome, and
  removing artifacts whose content lost its last reference (`Ingest`); and the decisions behind a video response — requeueing a missing hover
  preview, deriving the seek-preview state from its stored state and requeueing a `done`
  one whose sprite is missing or incomplete — plus assembling related videos, which for a
  folder-group member orders next/previous inside the group and leaves its members out of
  the related list (`Catalog`); and adding, replacing and removing media folders after the
  filesystem adapter has checked the path (`MediaFolders`); and holding the live-transcode
  video encoder choice with the startup encoder checks, run concurrently with a
  per-encoder time limit and kept in memory only, to decide the encoder actually used
  and to save a new choice (`TranscodeSettings`); and first-run setup,
  login verification with per-source throttling, issuing, checking and
  revoking login sessions, and issuing (`vvt_` plus 32 random bytes), listing, checking
  and revoking API tokens (`Auth`). It reaches storage, `ffmpeg`/`ffprobe` and generated files only
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

Each HTTP API boundary has a single source of truth. `api/openapi.yaml` is the
contract between the Go backend and the TypeScript frontend; both sides are
generated from it. `api/external-v1.yaml` is the versioned contract with external
tools (the external API under `/api/v1`); only Go is generated from it
(`internal/httpapi/extgen/`), because the SPA never calls it. Both are generated
by `task generate`, the generated files are version controlled, and CI fails when
regenerating them produces a diff.

`web/embed.go` is the one deliberate exception to the layering: Go's embed
directive cannot reference a parent directory, so the declaration that pulls
`web/dist` into the binary lives next to the SPA and `internal/httpapi/spa.go`
consumes it as an `fs.FS`.

## Web layer

The SPA under `web/src` is split by responsibility rather than by widget.

`web/src/api/` is the only place that talks to the server. `client.ts` wraps
`fetch` (turning a failed `fetch` into `NetworkFailed` and an error response into
`RequestFailed`) over the generated types in `web/src/api/gen/` (never hand-edited;
`task generate` rewrites them from `api/openapi.yaml`), `serverEvents.ts` shares one
`EventSource` on `/api/events` among its subscribers, `useVideos.ts` owns
paging and request cancellation for the library list and re-fetches a listed video in
place when a `video` event names it (its paging, per-item and per-group re-fetch,
list-data reducer and criteria keying live in `useVideoPages.ts`, `useItemRefresh.ts`,
`useGroupRefresh.ts`, `videosData.ts` and `videosCriteria.ts`). A re-fetched video
that is no longer its bundle's representative is dropped when the representative is
already listed; otherwise the list reloads from its first page, because only the
server knows whether the representative belongs to this list's folder and filters. Its items are `LibraryItem`s (a video or a folder
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
guest on an owner-only screen (`/settings`, `/tags`, `/duplicates`) to `/login?next=…`, and exposes the
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
primitives live in `web/src/ui/` and locale-independent formatting helpers (duration,
size, resolution) live in `web/src/lib/`. The seek-thumbnail sprite's frame selection and
sheet cropping live in `web/src/lib/seekSprite.ts`, so the player's seek bar and the
cards' scrub band pick the same frame for the same position, and the scrub band's hook
and parts (`useScrubPreview`, `ScrubBand`, `ScrubFrame`) live in
`web/src/ui/ScrubPreview.tsx`, shared by the video cards and the playback screen's related
videos and fetching the sprite through `web/src/api/client.ts`. User-facing text, the locale-dependent formatting
(numbers, dates, relative time, plurals) and the display of API errors live in
`web/src/i18n/`: an English catalog that components import statically as `t`, whose values
are the branded `UiText` type that `web/src/ui/` props require, and `errorText`, which
turns a `RequestFailed` (its `reason`, `code`, `limit` and `tagName`) or a `NetworkFailed`
into English instead of showing the server's or the browser's text. ESLint reports
Japanese or fixed text outside `web/src/i18n/`; the details are in
[docs/design-docs/i18n.md](docs/design-docs/i18n.md). The
video-list pieces the library and folder screens share (list criteria and their URL hook,
the condition labels and count summary, the video card, the empty/loading/error states and
the search, filter, sort and zoom controls) live in `web/src/videoList/`, which belongs to
neither screen, so neither screen imports from the other. `web/src/versions/` holds the
pieces for bundling videos as versions of one video that more than one screen uses: the
row's difference line (shared with the playback screen's versions list), the dialog
that picks the representative, which the library's selection bar and the candidates
screen open, and the owner-only candidates screen (`/duplicates`), which lists the pairs
a scan found to look like the same video, bundles a pair through that dialog or records
it as different videos, and refetches on the `scan` notification. `web/src/tags/` is the tag
admin screen (`/tags`): a list of every tag with its video count, an in-page name/synonym
search, create, rename and delete. `web/src/shell/navigation.ts` puts its sidebar entry
right after "フォルダ" (Folders), followed by the owner-only "Duplicates" entry. Every sidebar entry links to a working screen. The library, folder, settings, tag and candidates screens use the shell: `app/App.tsx`
puts `AppShell` around the `/`, `/folders/*`, `/settings`, `/tags` and `/duplicates` routes, and the
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
- Keep generated code (`internal/httpapi/gen/`, `internal/httpapi/extgen/`,
  `web/src/api/gen/`) generated; change `api/openapi.yaml` or
  `api/external-v1.yaml` instead.
