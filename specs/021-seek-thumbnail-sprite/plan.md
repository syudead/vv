# Implementation Plan: seek thumbnails as sprite sheets with caps on frame and file counts

> [!NOTE]
> The generation method was later updated by
> [seek sprite generation for long videos](../../docs/design-docs/seek-sprite-generation.md).
> The full decode described below is the initial implementation's decision. It now applies only
> to inputs that neither the index nor per-interval extraction can handle.

**Branch**: `feature/021-seek-thumbnail-sprite` | **Parent Issue**: #389

**Input**: The parent Issue. It is this feature's specification.

## Summary

Today each seek thumbnail is a JPEG every 5 s, stored as an individual file, and the player fetches
one per position change. The count grows with the video duration without a cap (1,440 files for
2 h). This feature replaces them with at most 6 sprite sheets per video (10 columns × 10 rows, at
most 600 frames) plus a layout that describes the arrangement.

- Pure functions in `internal/domain` derive the frame interval, frame count and sheet count from
  the video duration. Up to 50 min the interval stays at the current 5 s; only longer videos get a
  wider interval ([research.md R-1](research.md#r-1-at-most-600-frames-on-6-sheets-the-interval-widens-only-past-50-min)).
- Generation stays one full decode, as today, but the `fps` filter takes 1 frame per interval
  instead of `select`, and the `tile` filter arranges the frames in a grid. Intervals without a
  frame get the previous frame, and the last frame is cloned when the end has no video, so frame
  numbers keep matching playback positions ([R-2](research.md#r-2-frames-come-from-fps-tpad-trim-and-tile-in-one-decode)).
- The location stays `seek/<p>/<s>/` and holds the sheet JPEGs and the layout `sprite.json`. The
  presence of `sprite.json` marks completion. A location without it (including old individual
  JPEGs) is incomplete and gets rebuilt; publishing collects the old-format files
  ([R-3](research.md#r-3-the-location-stays-seek-spritejson-marks-completion)).
- `GET /api/videos/{id}/seek-thumbnail` returns the layout (JSON) instead of the `positionMs` JPEG,
  and `GET /api/videos/{id}/seek-thumbnail/{sheet}` returns a sheet
  ([contracts/seek-sprite-api.md](contracts/seek-sprite-api.md), [R-4](research.md#r-4-the-api-returns-a-json-layout-and-separate-jpeg-sheets)).
- The player fetches the layout once and each sheet once when needed, and switches frames within
  the same video without refetching. It crops frames at the layout size and never shows the
  neighboring frame ([R-5](research.md#r-5-the-player-fetches-each-sheet-once-and-crops-it-with-the-background)).
- A migration requeues `seek_thumbnail` jobs for existing videos, which rebuilds them as sprites
  without a re-import. Until the rebuild finishes they behave like today's `pending`: the time
  display, playback and seeking work ([R-6](research.md#r-6-a-migration-requeues-existing-individual-jpegs-for-rebuild)).
- Unchanged rules: reuse of generated files (videos with the same content are generated once),
  invalidation when the source is replaced or deleted, deletion of temporary files on
  interruption, and the same logical time for direct playback and live transcoding.

## Technical Context

**Canonical definitions**:

- Boundaries and dependency direction, ownership of generated files, per-stage workers:
  [ARCHITECTURE.md](../../ARCHITECTURE.md) (the "Generated files have one owner" and "Intended
  dependency direction" paragraphs)
- Current generation: [internal/media/seek_thumbnail.go](../../internal/media/seek_thumbnail.go)
  (the `select` and `scale` expressions, the 30 min cap). Location and reads:
  [internal/artifacts/store.go](../../internal/artifacts/store.go)
  (`PublishSeekThumbnails`, `SeekThumbnail`, `SeekThumbnailsAvailable`, `RemoveContent`).
  Interval constant: `domain.SeekThumbnailInterval`
  ([internal/domain/video.go](../../internal/domain/video.go))
- Jobs and state: [specs/020-seek-thumbnail-stage/data-model.md](../020-seek-thumbnail-stage/data-model.md)
  (the `seek_thumbnail` job, `videos.seek_thumbnail_state`, and `RequeueMissingSeekThumbnails`,
  which requeues a `done` video that lost its location), `Ingest.SeekThumbnails` and
  `Catalog.SeekThumbnailState`
  ([internal/app/ingest.go](../../internal/app/ingest.go), [internal/app/catalog.go](../../internal/app/catalog.go))
- Current HTTP contract: [api/openapi.yaml](../../api/openapi.yaml) (`getVideoSeekThumbnail`,
  `Video.seekThumbnailUrl`, `seekThumbnailState`),
  [internal/httpapi/seek_thumbnail.go](../../internal/httpapi/seek_thumbnail.go), the table of
  routes that also serve guests ([internal/httpapi/auth.go](../../internal/httpapi/auth.go)), cache
  directives for generated files
  ([specs/016-single-account-auth/contracts/guest-api.md §5](../016-single-account-auth/contracts/guest-api.md#5-cache-for-generated-media))
- Current player side: [web/src/player/seekPreview.ts](../../web/src/player/seekPreview.ts)
  (the 5 s bucket, request abort, the 5 s retry hold, `fetchSeekThumbnail`), mounting in
  [web/src/player/VideoPlayer.tsx](../../web/src/player/VideoPlayer.tsx), display rules:
  [specs/009-seek-thumbnail-preview/ui-design.md](../009-seek-thumbnail-preview/ui-design.md) and
  `.vv-seek-preview` in [web/src/index.css](../../web/src/index.css)
- The decision to use the source video's logical time for live transcoding too:
  `specs/009-seek-thumbnail-preview/plan.md` (Structural Decisions; removed from `main` as the Plan
  of a finished feature, it remains only in the git history)
- Precedents: the hover preview manifest (`preview/<p>/<s>.mp4.sha256`, `internal/artifacts`), the
  requeue migration for existing videos
  [00014_seek_thumbnail_stage.sql](../../internal/store/migrations/00014_seek_thumbnail_stage.sql),
  and the procedure for measuring generation changes
  [docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md) with
  [scripts/previewbench](../../scripts/previewbench/main.go)
- Check entry points: [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`,
  `task generate`, `task test-e2e`)

**Feature-specific context**:

- No new dependency. The ffmpeg filters `fps`, `tpad`, `trim` and `tile` were verified with the
  bundled ffmpeg (6.1) ([research.md R-2](research.md#r-2-frames-come-from-fps-tpad-trim-and-tile-in-one-decode)).
- The SQLite schema does not change. Migration `00015` only returns existing videos'
  `seek_thumbnail_state` to `pending` and queues jobs
  ([R-6](research.md#r-6-a-migration-requeues-existing-individual-jpegs-for-rebuild)).
- The cap is "at most 600 frames and at most 6 sheets per video"; a 2 h video gets 600 frames at
  12 s intervals. The numbers are justified in [R-1](research.md#r-1-at-most-600-frames-on-6-sheets-the-interval-widens-only-past-50-min).
- The response type of `GET /api/videos/{id}/seek-thumbnail` changes from JPEG to JSON. The SPA
  ships inside the binary and updates with it, so compatibility with the old SPA is not kept (as
  with earlier contract changes).

## Constitution Check

| Principle | Result | Evidence |
| --- | --- | --- |
| Dependency direction (ARCHITECTURE.md "Intended dependency direction") | Pass | The layout rule is a pure function in `internal/domain`; `internal/media` only maps it to ffmpeg arguments; `internal/artifacts` owns the location, manifest and reads; `internal/app` and `internal/httpapi` use them through interfaces they declare. |
| Ownership of generated files (ARCHITECTURE.md "Generated files have one owner") | Pass | The location directory stays `seek/<p>/<s>/`. The content format changes, but only `internal/artifacts` recognizes and collects old-format files. In-progress output stays under `.tmp` as today and is renamed as a whole directory, so it is never served (Requirement 8). |
| Index versus user data (ARCHITECTURE.md "Two kinds of data") | Pass | The migration touches only the rebuildable `videos.seek_thumbnail_state` and `jobs`. |
| API source of truth (ARCHITECTURE.md) | Pass | The layout schema and routes are added to `api/openapi.yaml`; `task generate` produces the Go and TypeScript code. |
| Guest responses and caching (guest-api.md §5) | Pass | The layout and sheets both return `private, no-cache` and `ETag`, and join the table of routes that also serve guests. |
| Docs change in the same PR as behavior (core-beliefs.md) | Pass | The unit that switches storage and serving updates the ARCHITECTURE.md paragraph on generated files; the migration unit updates the tech stack selection and `specs/009` (Requirement 10). |

The result is the same after Phase 1. No violation goes into Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/021-seek-thumbnail-sprite/
├── plan.md                        # This file
│                                  # No spec.md — the parent Issue is the specification
├── research.md                    # Decisions: caps and interval, frame selection, location, API shape, cropping, migration
├── quickstart.md                  # 2 h, portrait and live transcoding inputs, and the before/after measurement
└── contracts/
    └── seek-sprite-api.md         # Layout and sheet routes, the changed meaning of Video.seekThumbnailUrl
```

- No `data-model.md`. SQLite gains no column or table; the layout is a file stored with the
  generated files, and [research.md R-3](research.md#r-3-the-location-stays-seek-spritejson-marks-completion) owns its shape.
- No `ui-design.md`. The preview's look (size, position, time label) does not change, and the
  cropping rule is in [R-5](research.md#r-5-the-player-fetches-each-sheet-once-and-crops-it-with-the-background).

### Source Code

**Affected boundaries**:

| Path | Change |
| --- | --- |
| `internal/domain` | Pure functions and constants that derive the sprite layout (interval, frame count, columns, rows, sheet count) from the duration. `SeekThumbnailInterval` becomes the minimum interval. |
| `internal/media` | Writes the sheets with one ffmpeg run using `fps`, `tpad`, `trim`, `scale` and `tile`. The measurement boundary `GenerateSeekThumbnailSet`. |
| `internal/artifacts` | Publishes sheets and `sprite.json` (frame size read from the sheet JPEG), decides completion, reads the layout and sheets, collects old-format locations. |
| `internal/app` | `Ingest.SeekThumbnails` derives the layout from the duration and passes it to generation. `Catalog.SeekThumbnailState` takes the changed completion check as is. |
| `api/openapi.yaml`, `internal/httpapi` | Layout response, sheet route, the table of routes that also serve guests. |
| `web/src/player`, `web/src/api` | Fetching the layout and sheets, frame cropping, waits in `web/e2e`. |
| `internal/store/migrations` | Requeue for existing videos. |
| `scripts/previewbench`, `docs/how-to/preview-benchmark.md` | Measurement of seek thumbnail generation. |
| `ARCHITECTURE.md`, `docs/design-docs/tech-stack-selection.md`, `specs/009-seek-thumbnail-preview` | Descriptions of the method. |

**New paths**: `internal/store/migrations/00015_seek_thumbnail_sprite.sql`.

**Structure decision**: Follow the existing layout ([ARCHITECTURE.md](../../ARCHITECTURE.md)).

## Implementation Work

### Measure seek thumbnail generation with previewbench

**Scope**:

- Add the kind to measure (animated preview or seek thumbnail) to `scripts/previewbench`.
- As the seek measurement boundary, add
  `GenerateSeekThumbnailSet(ctx, videoPath, outputDir string, durationMs int64) error` to
  `internal/media`. For now it writes individual JPEGs to `outputDir` with the current
  generation.
- The seek kind of previewbench calls only this function and prints the file count and total
  bytes of `outputDir` in addition to wall clock time and peak memory. Later units change only the
  body of this function and do not touch `scripts/previewbench`.
- Add the seek measurement and the PR table format to
  [docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md)
  ([quickstart.md](quickstart.md) §1). Generation itself does not change.

**Dependencies**: None.

**Acceptance**:

- With the current generation on the 2 h input, `go run ./scripts/previewbench -kind seek <video>`
  prints wall clock time, peak memory, file count and total bytes, and leaves no temporary output
  after exit.
- The `scripts/previewbench` tests check kind parsing and the seek totals.
- `task check` and `task check-docs` pass.

### Build the seek thumbnail layout rule and sprite sheet generation

**Scope**:

- Put the layout rule of [research.md R-1](research.md#r-1-at-most-600-frames-on-6-sheets-the-interval-widens-only-past-50-min) in `internal/domain`.
- Add a generation function to `internal/media` that writes sheets with the ffmpeg arguments of
  [R-2](research.md#r-2-frames-come-from-fps-tpad-trim-and-tile-in-one-decode). It takes the layout, writes the sheets in one ffmpeg run,
  and is callable from `Assets` too.
- Change the body of `GenerateSeekThumbnailSet` to derive the layout from the duration and call
  this function.
- Storage, serving and the player do not change, so the playback screen behaves the same in this
  unit.

**Dependencies**: `Measure seek thumbnail generation with previewbench`.

**Acceptance**:

- `internal/domain` tests check: 50 min or less keeps the 5 s interval; 2 h gives 12 s, 600 frames
  and 6 sheets; no duration exceeds 600 frames or 6 sheets; a short video with a single frame.
- `internal/media` tests (when ffmpeg is present) check, with a test input that draws the time:
  - the times of the first, middle and last frames lie in their intervals;
  - with video shorter than the container, the last frame shows the last scene;
  - an input shorter than half the interval (1 s) gives a one-frame sheet;
  - with a portrait input, frames keep the aspect ratio and have the same size within the video.
- The PR shows a table of generation time, peak memory, file count and total size before and
  after, on the same 2 h and 2 min inputs ([quickstart.md](quickstart.md) §1).
- `task check` and `task check-docs` pass.

### Store and serve seek thumbnails as sprite sheets, and crop them in the player

**Scope**:

- Move `internal/artifacts` to the location, `sprite.json`, completion check and old-format
  collection of [R-3](research.md#r-3-the-location-stays-seek-spritejson-marks-completion). `Ingest.SeekThumbnails` in
  `internal/app` derives the layout from the duration and passes it to the previous unit's
  generation function.
- Delete the individual JPEG generation in `internal/media` that nothing calls any more
  (`GenerateSeekThumbnails` and `Assets.SeekThumbnails`).
- Change `api/openapi.yaml` per [contracts/seek-sprite-api.md](contracts/seek-sprite-api.md), run
  `task generate`, and update the layout and sheet responses in `internal/httpapi` and the table
  in `auth.go`.
- Per [R-5](research.md#r-5-the-player-fetches-each-sheet-once-and-crops-it-with-the-background), make `web/src/player/seekPreview.ts`
  fetch the layout once, fetch each needed sheet once and hold it as an object URL, and crop
  frames at the layout size. Drop the 5 s bucket and the one-by-one fetch of
  `fetchSeekThumbnail`; adapt the fetch functions in `web/src/api/client.ts` to the layout and
  sheets.
- `VideoPlayer.tsx` remounts when `seekThumbnailState` changes. The look (size, position, time
  label) does not change.
- Only this unit changes `web/e2e/playback.e2e.ts` (waits and the seek preview tests), and it
  updates the ARCHITECTURE.md paragraphs on generated files and workers.
- The API and the player switch in the same unit: the current player cannot read the new
  response, and splitting them would break the seek preview on the feature branch in between.

**Dependencies**: `Build the seek thumbnail layout rule and sprite sheet generation`.

**Acceptance**:

- `internal/artifacts` tests check: a location without `sprite.json` is incomplete; publishing
  deletes old individual JPEGs and replaces them with sheets and `sprite.json`; interruption leaves
  nothing in the temporary location; `RemoveContent` deletes the location.
- `internal/httpapi` tests check: the layout returns in the contract shape with
  `private, no-cache` and `ETag`, and `If-None-Match` gives 304; an out-of-range sheet number
  gives 404; generation in progress gives 409.
- Web unit tests check:
  - the position-to-frame-and-sheet rule uses the layout interval and does not assume 5 s;
  - moving within a sheet fetches nothing, and moving across sheets fetches the next sheet once;
  - returning to a sheet whose fetch has not finished sends no second fetch;
  - after a failed layout or sheet fetch the time display continues and a retry follows after
    5 s;
  - unmount aborts in-flight fetches and releases the object URLs.
- The `task test-e2e` playback test checks that direct playback and live transcoding show the same
  frame at the same position.
- The screen changes, so with the 2 h time-drawing input and the portrait input
  ([quickstart.md](quickstart.md) §2), confirm at 360px, 768px and 1280px and record in the PR:
  the times of frames near the start, middle and end lie in their intervals; no neighboring frame
  shows, even for portrait; continuous pointer movement on the seek bar sends no additional sheet
  request for the same video in the browser network log.
- Confirm that assistive technology still does not announce the preview.
- `task check` and `task check-docs` pass.

### Rebuild existing seek thumbnails as sprites, and align the tech stack and 009 docs

**Scope**:

- Per [research.md R-6](research.md#r-6-a-migration-requeues-existing-individual-jpegs-for-rebuild), migration
  `00015_seek_thumbnail_sprite.sql` returns videos with `seek_thumbnail_state = done` to `pending`,
  and queues a `seek_thumbnail` job for each analyzed video with a location and no unfinished
  `seek_thumbnail` job.
- Align the seek preview description in `docs/design-docs/tech-stack-selection.md` with sprite
  sheets.
- In `specs/009-seek-thumbnail-preview`, replace the descriptions of the 5 s interval, individual
  JPEGs and `positionMs` in `plan.md`, `research.md`, `contracts/seek-thumbnail.md` and
  `quickstart.md` with references to this feature's contract (Requirement 10).

**Dependencies**: `Store and serve seek thumbnails as sprite sheets, and crop them in the player`.

**Acceptance**:

- `internal/store` tests check that `00015` returns `done` videos to `pending` and queues
  `seek_thumbnail`, queues nothing for `failed` videos or videos with an unfinished job, and that
  `up` / `down` pass (`task migrations-check`).
- Restarting `task preview` with a data directory holding the old individual JPEGs shows remaining
  seek thumbnails in the processing status. When they finish, only sheets and `sprite.json`
  remain in `seek/<p>/<s>/`, and playback, seeking and the time display work on the playback
  screen throughout ([quickstart.md](quickstart.md) §3).
- `task check` and `task check-docs` pass.
