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
carries its representative location and seek-preview state, and
`/api/videos/{id}/related`, `/probe` and `/open` return related videos, retry a failed
metadata read, and open the file in the server PC's default app), media-folder settings and
server-side directory picker APIs, the read-only folder browsing API
(`/api/folders*`), byte-range streaming,
thumbnails, playback progress, and the SPA embedded from `web/dist`.

Both video lists, the library (`GET /api/videos`) and a folder
(`GET /api/folders/{rootId}/videos`, direct children by default or the whole
subtree with `scope=subtree`), accept the same search expression (`query`),
watch-state and playable filters, thirteen sort orders and a shuffle `seed`.
`internal/httpapi` validates those parameters at the entry and hands them to
the store as `domain.VideoQuery` / `domain.FolderVideoQuery`; `total` counts
every match after all of them apply, and each item carries the folder of the
location it was listed from (`Video.folder`, built from the registered media
folders with `domain.LocateVideoFolder`)
([specs/013-library-search/contracts/list-api.md](specs/013-library-search/contracts/list-api.md)).

`internal/scanner` walks a snapshot of the media folders stored in SQLite when a user starts
a scan. It identifies files by content
(`sha256` over the first and last 1MiB plus the size) so moves and renames do
not duplicate rows, and queues the heavy work. Only a user-started scan walks the
media folders; nothing reads the whole library at startup or after a scan.
`internal/jobs` runs one in-process worker per ingest stage — probe, thumbnail, preview —
each claiming only its own kind of job from the persistent `jobs` queue, one at a time,
and driving the `internal/media` adapters
(`ffprobe` for metadata, `ffmpeg` for one library thumbnail, five-second seek-preview frames,
and a content-keyed hover-preview clip per video). A worker sleeps while its queue is
empty: `internal/store` reports every committed enqueue, and `cmd/mdm` wakes the worker
for that stage, so no worker polls the queue. A thumbnail job is not claimed until its
video's probe has finished, because the frame position depends on the duration; the probe
worker wakes the thumbnail worker when it records a result. Interrupted scans are closed,
running jobs are requeued, and the single `.tmp` directory that holds in-progress
generation output is removed at the next startup. When a video row is deleted (a scan finds its last
location gone, its content changes, or its media folder is removed or replaced),
`internal/store` reports the released content keys after commit, and `cmd/mdm` removes
that content's thumbnail, seek frames and hover preview unless another video still
references it; nothing else sweeps the thumbnails directory. A hover preview whose file
is gone is repaired when it is found: the video API already checks the file before
exposing `previewUrl`, and when a `done` preview is missing it sets the video back to
`pending` and queues a preview job in one transaction, once per loss.

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

`internal/opener` launches the operating system's default app for a video's
representative location (`explorer.exe`, `open` or `xdg-open`). It is kept apart from
`internal/media`, which is the entry point for `ffmpeg`/`ffprobe`, and it resolves the
command once at startup; on Linux and similar systems it also requires `DISPLAY` or
`WAYLAND_DISPLAY`, so containers and headless servers report it as unavailable. The
open route only accepts requests whose remote address and `Host` are loopback, and it
never takes a path from the request.

Shutdown closes the `/api/events` streams, drains in-flight requests within a 10 second
grace period, then stops the scanner and the workers so a running job returns to the queue.

Two kinds of data live in SQLite and they are not equivalent: `videos`,
`video_locations` (including its per-location search keys), `location_search_fts`,
`jobs`, `scans`, thumbnail files, and hover-preview MP4/manifest pairs are a rebuildable index
(deleting them costs a rescan), while `playback_progress` is user data that
cannot be reconstructed. That is why playback positions are keyed by the
content identifier rather than by `videos.id`, and why that table carries no
foreign key to `videos`.

Search matches a per-location `search_key` that Go builds from the title and the
path below the registered media folder, folded with `domain.FoldForMatch`, and
indexed by the trigram FTS5 table `location_search_fts`. SQL cannot express that
folding, so startup refreshes every location whose `search_version` is older than
`domain.SearchKeyVersion` right after `store.Migrate` and before jobs or HTTP start,
and aborts startup if that fails
(`specs/013-library-search/data-model.md` §5).

Not built yet: authentication, subtitles, and multi-user support. Browser-incompatible
video can be transcoded to a request-scoped fragmented MP4 stream; transcoded output is
not persisted.

## Intended dependency direction

`cmd -> internal/{httpapi,store,media,opener,scanner,jobs} -> internal/domain`, one way
only. `internal/domain` holds the domain model and use cases and must not depend
on `net/http`, `database/sql`, or `os/exec`. This constraint is enforced
mechanically with golangci-lint's depguard in CI.

The depguard rules live in [.golangci.yml](.golangci.yml) and also deny the
SQLite driver and every other `internal/*` package from `internal/domain`. Each
rule carries the reason in its message, so a violation explains itself from the
`task lint` output alone.

The sibling packages under `internal/` do not import each other either, and
that is not mechanically enforced — it is a convention the code keeps by
declaring what it needs. `internal/scanner`, `internal/jobs` and
`internal/httpapi` each define the interfaces they consume (an index to write
to, a queue to claim from, a library to query), `internal/store` happens to
satisfy them, and `cmd/mdm` is the only place that knows which concrete type
goes where. The values crossing those boundaries (`domain.VideoFile`,
`domain.Job`, `domain.VideoQuery`, …) live in `internal/domain`, which is why
neither side needs the other.

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
place when a `video` event names it, `useVideoDetail.ts`
fetches one video for the playback screen and re-fetches it when a `video` event names
it or the event stream reconnects, and `listSnapshot.ts`
holds the in-memory snapshot that lets the list restore its position after a
round trip to the playback screen. Pages and components do not call `fetch`
themselves, so how the server is reached stays changeable in one place.
The list's conditions (search terms, watch state, playable-only, sort and the shuffle
`seed`) live in the URL; `web/src/library/listCriteria.ts` converts between the URL and
the criteria `useVideos` sends, and the server applies every condition, so the page
neither filters loaded pages nor reads ahead to find matches.

`web/src/shell/` holds the responsive top bar, sidebar, scan state, and the
frame around a screen. `web/src/library/`, `web/src/folders/`, `web/src/settings/`, and
`web/src/player/` own their respective product flows, while reusable primitives live in
`web/src/ui/` and formatting helpers live in `web/src/lib/`. The library, folder and settings
screens use the shell: `app/App.tsx` puts `AppShell` around the `/`, `/folders/*` and
`/settings` routes, and the playback screen
(`/videos/:id`) deliberately gets no shell at all, because it is a
two-pane screen of its own: the player with the title, a property strip and the
file location on the left, related videos on the right, and a close button (×, or
Esc) that returns to the list the screen was opened from. Keeping that choice to
the one routing decision is what lets the shell stay ignorant of which screen it
is framing. Inside `web/src/player/`, video.js owns only the control bar; ingest
stages, read and playback failures, the ended prompt and the touch controls are
React layers stacked in one container above the player, and keyboard shortcuts are
handled page-wide rather than by video.js. The composition is recorded in
[docs/design-docs/library-ui.md](docs/design-docs/library-ui.md).
The shell exposes the library, the folder browser and media-folder settings as routes.
The folder browser reuses the library's video card and paging (`useVideos` takes the
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
