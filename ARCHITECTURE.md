# Architecture

This repository targets a self-hosted media data management (MDM) system: it
indexes video files on local storage and plays them back in a browser. The
core is in place — a single Go binary that scans a media directory, serves the
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

In place today: `cmd/mdm` reads `MDM_*` environment variables, checks that
`ffprobe`/`ffmpeg` are on `PATH`, opens SQLite under `MDM_DATA_DIR` and applies
embedded goose migrations at startup, then starts the job worker and (unless
`MDM_SCAN_ON_START=false`) one background scan. It serves `GET /api/health`,
the video library API (`/api/videos*`, `/api/scans*`), byte-range streaming,
thumbnails, playback progress, and the SPA embedded from `web/dist`.

`internal/scanner` walks `MDM_MEDIA_DIR`, identifies files by content
(`sha256` over the first and last 1MiB plus the size) so moves and renames do
not duplicate rows, and queues the heavy work. `internal/jobs` runs a single
serial in-process worker that drives the `internal/media` adapters
(`ffprobe` for metadata, `ffmpeg` for one thumbnail per video); interrupted
jobs are requeued at the next startup. Streaming delegates ranges to
`http.ServeContent` and only opens paths that resolve inside `MDM_MEDIA_DIR`.

Shutdown drains in-flight requests within a 10 second grace period, then stops
the scanner and the worker so a running job returns to the queue.

Two kinds of data live in SQLite and they are not equivalent: `videos`,
`videos_fts`, `jobs`, `scans` and the thumbnail files are a rebuildable index
(deleting them costs a rescan), while `playback_progress` is user data that
cannot be reconstructed. That is why playback positions are keyed by the
content identifier rather than by `videos.id`, and why that table carries no
foreign key to `videos`.

Not built yet: authentication, subtitles, transcoding for formats the browser
cannot play, and multi-user support.

## Intended dependency direction

`cmd -> internal/{httpapi,store,media,scanner,jobs} -> internal/domain`, one way
only. `internal/domain` holds the domain model and use cases and must not depend
on `net/http`, `database/sql`, or `os/exec`. This constraint is enforced
mechanically with golangci-lint's depguard in CI.

The depguard rules live in [.golangci.yml](.golangci.yml) and also deny the
SQLite driver and every other `internal/*` package from `internal/domain`. Each
rule carries the reason in its message, so a violation explains itself from the
`make lint` output alone.

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
generated from it (`make generate`), the generated files are version
controlled, and CI fails when regenerating them produces a diff.

`web/embed.go` is the one deliberate exception to the layering: Go's embed
directive cannot reference a parent directory, so the declaration that pulls
`web/dist` into the binary lives next to the SPA and `internal/httpapi/spa.go`
consumes it as an `fs.FS`.

## Principles

- Make module boundaries explicit and mechanically enforceable where possible.
- Keep dependencies directed from product-facing layers toward stable domain
  interfaces.
- Capture consequential design decisions in `docs/design-docs/`.
- Generate volatile reference material into `docs/generated/`.
