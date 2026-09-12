# Architecture

This repository targets a self-hosted media data management (MDM) system: it
indexes video files on local storage and plays them back in a browser. The
Phase 0 skeleton is in place — a single Go binary that serves the JSON API and
the embedded React SPA, backed by SQLite — and the remaining components are
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

In place today (Phase 0): `cmd/mdm` reads `MDM_*` environment variables, checks
that `ffprobe`/`ffmpeg` are on `PATH`, opens SQLite under `MDM_DATA_DIR` and
applies embedded goose migrations at startup, then serves `GET /api/health`
plus the SPA embedded from `web/dist`. Shutdown drains in-flight requests
within a 10 second grace period.

Not built yet: byte-range streaming, the scanner, the job worker, the
`ffmpeg`/`ffprobe` adapters beyond their startup existence check, and
authentication. `internal/scanner` and `internal/jobs` exist as declared
boundaries only.

## Intended dependency direction

`cmd -> internal/{httpapi,store,media,scanner,jobs} -> internal/domain`, one way
only. `internal/domain` holds the domain model and use cases and must not depend
on `net/http`, `database/sql`, or `os/exec`. This constraint is enforced
mechanically with golangci-lint's depguard in CI.

The depguard rules live in [.golangci.yml](.golangci.yml) and also deny the
SQLite driver and every other `internal/*` package from `internal/domain`. Each
rule carries the reason in its message, so a violation explains itself from the
`make lint` output alone.

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
