# Implementation Plan: Seek thumbnails as sprite sheets, with limits on frame and file count

> The generation method was later updated by
> [seek sprite generation for long videos](../../docs/design-docs/seek-sprite-generation.md).
> The full-video decode described below is the decision of the initial
> implementation; it is now used only for inputs that can be built neither from
> the index nor by per-segment extraction.

**Branch**: `feature/021-seek-thumbnail-sprite` | **Parent Issue**: #389

**Input**: The parent Issue. It is this feature's specification.

## Summary

Seek thumbnails are currently stored as one JPEG per 5 seconds, each its own
file, and the player fetches them one at a time whenever the position changes.
Their number grows with the video's length without limit (1,440 files for 2
hours). This feature replaces them with at most 6 sprite sheets per video (10
columns × 10 rows, at most 600 frames) plus layout information that describes
the arrangement.

- Pure functions in `internal/domain` derive the frame interval, frame count
  and sheet count from the video length. Up to 50 minutes the interval stays at
  today's 5 seconds; only longer videos get a wider interval
  ([research.md R-1](research.md#r-1-limits-and-interval-rule)).
- Generation is still one full-video decode, but instead of `select` the `fps`
  filter takes one frame per interval and the `tile` filter lays them out in a
  grid. A range with no frames is filled with the previous frame, and the last
  frame is cloned when there is no video at the end, so frame numbers keep
  matching playback positions ([R-2](research.md#r-2-frame-selection)).
- The location stays `seek/<p>/<s>/` and holds the sheet JPEGs and the layout
  information `sprite.json`. The completion marker is the presence of
  `sprite.json`; a location without it (including old-format individual JPEGs)
  is treated as incomplete and rebuilt, and old-format files are reclaimed at
  publish time ([R-3](research.md#r-3-storage-location-and-completion-marker)).
- `GET /api/videos/{id}/seek-thumbnail` returns the layout information (JSON)
  instead of a `positionMs` JPEG, and sheets are returned by
  `GET /api/videos/{id}/seek-thumbnail/{sheet}`
  ([contracts/seek-sprite-api.md](contracts/seek-sprite-api.md),
  [R-4](research.md#r-4-api-shape)).
- The player fetches the layout information once and each sheet once when it is
  needed, and switches frames without refetching for the same video. It crops
  frames by the size in the layout information and never shows the
  neighbouring frame
  ([R-5](research.md#r-5-player-fetching-and-frame-cropping)).
- For existing videos, a migration requeues `seek_thumbnail` jobs, rebuilding
  them as sprites without a re-ingest. Until the rebuild finishes they behave
  like today's `pending`: the time display, playback and seeking work
  ([R-6](research.md#r-6-migration-from-existing-individual-jpegs)).
- Unchanged: reuse of generated files (content shared between videos is
  generated once), invalidation when the source video is replaced or deleted,
  removal of temporary files on interruption, and the rule that direct delivery
  and live transcoding use the same logical time.

## Technical Context

**Canonical definitions**:

| Topic | Source |
| --- | --- |
| Boundaries and dependency direction, ownership of generated files, per-stage workers | [ARCHITECTURE.md](../../ARCHITECTURE.md) (paragraphs "Generated files have one owner" and "Intended dependency direction") |
| Current generation | [internal/media/seek_thumbnail.go](../../internal/media/seek_thumbnail.go) (the `select` and `scale` expressions, the 30-minute limit) |
| Storage and reading | [internal/artifacts/store.go](../../internal/artifacts/store.go) (`PublishSeekThumbnails`, `SeekThumbnail`, `SeekThumbnailsAvailable`, `RemoveContent`) |
| Interval constant | `domain.SeekThumbnailInterval` ([internal/domain/video.go](../../internal/domain/video.go)) |
| Jobs and state | [specs/020-seek-thumbnail-stage/data-model.md](../020-seek-thumbnail-stage/data-model.md) (`seek_thumbnail` job, `videos.seek_thumbnail_state`, `RequeueMissingSeekThumbnails` for a `done` that lost its files); `Ingest.SeekThumbnails` and `Catalog.SeekThumbnailState` ([internal/app/ingest.go](../../internal/app/ingest.go), [internal/app/catalog.go](../../internal/app/catalog.go)) |
| Current HTTP contract | [api/openapi.yaml](../../api/openapi.yaml) (`getVideoSeekThumbnail`, `Video.seekThumbnailUrl`, `seekThumbnailState`), [internal/httpapi/seek_thumbnail.go](../../internal/httpapi/seek_thumbnail.go), the table of paths also returned to guests ([internal/httpapi/auth.go](../../internal/httpapi/auth.go)), cache directives for generated files ([specs/016-single-account-auth/contracts/guest-api.md §5](../016-single-account-auth/contracts/guest-api.md#5-cache-of-generated-files)) |
| Current player side | [web/src/player/seekPreview.ts](../../web/src/player/seekPreview.ts) (5-second buckets, request abort, 5-second retry hold, `fetchSeekThumbnail`), mounting in [web/src/player/VideoPlayer.tsx](../../web/src/player/VideoPlayer.tsx) |
| Display rules | [specs/009-seek-thumbnail-preview/ui-design.md](../009-seek-thumbnail-preview/ui-design.md), `.vv-seek-preview` in [web/src/index.css](../../web/src/index.css) |
| Using the source video's logical time with live transcoding too | `specs/009-seek-thumbnail-preview/plan.md` (Structural Decisions; removed from `main` as the Plan of a finished feature, remains only in git history) |
| Precedents | The hover preview manifest (`preview/<p>/<s>.mp4.sha256`, `internal/artifacts`); the requeue migration for existing videos [00014_seek_thumbnail_stage.sql](../../internal/store/migrations/00014_seek_thumbnail_stage.sql); the procedure for measuring a generation change, [docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md) and [scripts/previewbench](../../scripts/previewbench/main.go) |
| Check entry points | [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`, `task generate`, `task test-e2e`) |

**Feature-specific context**:

- No dependency is added. ffmpeg's `fps`, `tpad`, `trim` and `tile` were
  confirmed to work with the bundled ffmpeg (6.1)
  ([research.md R-2](research.md#r-2-frame-selection)).
- The SQLite schema does not change. Migration `00015` only resets existing
  videos' `seek_thumbnail_state` to `pending` and queues jobs
  ([R-6](research.md#r-6-migration-from-existing-individual-jpegs)).
- The limit is "at most 600 frames and 6 sheets per video"; a 2-hour video gets
  600 frames at a 12-second interval. The numbers are justified in
  [R-1](research.md#r-1-limits-and-interval-rule).
- The response type of `GET /api/videos/{id}/seek-thumbnail` changes from JPEG
  to JSON. The SPA ships inside the binary and updates with it, so
  compatibility with an old SPA is not kept (the same treatment as earlier
  contract changes).

## Constitution Check

| Gate | Verdict |
| --- | --- |
| **Dependency direction** (ARCHITECTURE.md, "Intended dependency direction") | Pass. The layout rule is a pure function in `internal/domain`; `internal/media` only maps it to ffmpeg arguments; `internal/artifacts` owns the location, the manifest and reading; `internal/app` and `internal/httpapi` use them through interfaces they declare themselves. |
| **Ownership of generated files** (ARCHITECTURE.md, "Generated files have one owner") | Pass. The location directory stays `seek/<p>/<s>/`. The content format changes, but only `internal/artifacts` recognises and reclaims old-format files. Files being generated sit under `.tmp` as today and the directory is renamed as a whole, so they are never served (requirement 8). |
| **Index versus user data** (ARCHITECTURE.md, "Two kinds of data") | Pass. The migration touches only the rebuildable `videos.seek_thumbnail_state` and `jobs`. |
| **API source of truth** (ARCHITECTURE.md) | Pass. The layout information schema and paths are added to `api/openapi.yaml`, and Go and TypeScript are generated by `task generate`. |
| **Guest responses and caching** (guest-api.md §5) | Pass. Layout information and sheets are both returned with `private, no-cache` and `ETag`, and are added to the table of paths also returned to guests. |
| **Documents are fixed in the same PR as the change** (core-beliefs.md) | Pass. The generated-files paragraph of ARCHITECTURE.md is fixed by the unit that switches storage and delivery; the tech stack selection and the `specs/009` text are fixed by the migration unit (requirement 10). |

The verdicts are the same after Phase 1. There is no violation for Complexity
Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/021-seek-thumbnail-sprite/
├── plan.md                        # This file
│                                  # No spec.md — the parent Issue is the specification
├── research.md                    # Decisions on limits and interval, frame selection, location, API shape, cropping, migration
├── quickstart.md                  # 2-hour, portrait and live-transcode inputs, and the before/after measurement procedure
└── contracts/
    └── seek-sprite-api.md         # Layout information and sheet paths, the change in meaning of Video.seekThumbnailUrl
```

No `data-model.md`: SQLite gains no column or table, and the layout information
is a file stored with the generated files, whose shape is in
[research.md R-3](research.md#r-3-storage-location-and-completion-marker). No
`ui-design.md`: the preview's look (size, position, time display) does not
change, and the cropping rule is in
[R-5](research.md#r-5-player-fetching-and-frame-cropping).

### Source Code

**Affected boundaries**:

| Boundary | What changes |
| --- | --- |
| `internal/domain` | Pure functions and constants that derive the sprite layout (interval, frame count, columns, rows, sheet count) from the video length. `SeekThumbnailInterval` becomes the minimum interval. |
| `internal/media` | One ffmpeg run with `fps`, `tpad`, `trim`, `scale` and `tile` writes the sheets. The measurement boundary `GenerateSeekThumbnailSet`. |
| `internal/artifacts` | Publishing sheets and `sprite.json` (frame size read from the sheet JPEG), the completion check, reading layout information and sheets, reclaiming old-format locations. |
| `internal/app` | `Ingest.SeekThumbnails` derives the layout from the video length and passes it to generation. `Catalog.SeekThumbnailState` takes the changed completion check as is. |
| `api/openapi.yaml`, `internal/httpapi` | The layout information response, the sheet path, the table of paths also returned to guests. |
| `web/src/player`, `web/src/api` | Fetching layout information and sheets, cropping frames, the waits in `web/e2e`. |
| `internal/store/migrations` | Requeueing existing videos. |
| `scripts/previewbench`, `docs/how-to/preview-benchmark.md` | Measuring seek thumbnail generation. |
| `ARCHITECTURE.md`, `docs/design-docs/tech-stack-selection.md`, `specs/009-seek-thumbnail-preview` | Descriptions of the method. |

**New paths**: `internal/store/migrations/00015_seek_thumbnail_sprite.sql`.

**Structure decision**: Follows the existing layout
([ARCHITECTURE.md](../../ARCHITECTURE.md)).

## Implementation Work

The units depend on each other in a chain:

```mermaid
graph LR
  bench[Measure seek thumbnail generation in previewbench] --> gen[Seek thumbnail layout rule and sprite sheet generation]
  gen --> serve[Store and serve seek thumbnails as sprite sheets and crop them in the player]
  serve --> migrate[Rebuild existing seek thumbnails as sprites and align the tech stack and 009 docs]
```

### Measure seek thumbnail generation in previewbench

**Scope**: Add a kind of target to `scripts/previewbench` (animated preview or
seek thumbnail). As the measurement boundary for seek thumbnails, add
`GenerateSeekThumbnailSet(ctx, videoPath, outputDir string, durationMs int64) error`
to `internal/media`. For now it writes individual JPEGs to `outputDir` with the
current generation; the seek kind of previewbench calls only this function and
prints, besides wall-clock time and peak memory, the file count and total bytes
in `outputDir`. Later units change only this function's body and do not touch
`scripts/previewbench`. Add to
[docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md) how
to measure seek thumbnails and the table format to record in the PR
([quickstart.md](quickstart.md) §1). Generation itself does not change.

**Dependencies**: None.

**Acceptance**: `go run ./scripts/previewbench -kind seek <video>` prints
wall-clock time, peak memory, file count and total bytes for the 2-hour input
with the current generation, and no temporary output remains afterwards. Tests
in `scripts/previewbench` check the parsing of the kind and the seek
aggregation. `task check` and `task check-docs` pass.

### Seek thumbnail layout rule and sprite sheet generation

**Scope**: Put the layout rule of
[research.md R-1](research.md#r-1-limits-and-interval-rule) in
`internal/domain`, and add to `internal/media` a generation function that
writes sheets with the ffmpeg arguments of [R-2](research.md#r-2-frame-selection)
(it takes the layout and writes the sheets in one ffmpeg run, in a form also
callable from `Assets`). Change the body of `GenerateSeekThumbnailSet` to derive
the layout from the video length and call this generation function. Storage,
delivery and the player do not change, so the playback screen behaves the same
after this unit.

**Dependencies**: `Measure seek thumbnail generation in previewbench`.

**Acceptance**:

- Tests in `internal/domain` check: the interval stays 5 seconds up to 50
  minutes; 2 hours gives a 12-second interval, 600 frames and 6 sheets; no
  length exceeds 600 frames or 6 sheets; a short video with a single frame.
- Tests in `internal/media` (when ffmpeg is available) check, on a test input
  that draws the time: the times of the first, middle and last frames lie in
  their ranges; for an input whose video is shorter than the container, the
  last frame is the last scene; an input shorter than half an interval (1
  second) produces a sheet with one frame; for a portrait input, frames keep
  their aspect ratio and all frames of one video have the same size.
- The PR carries the table of generation time, peak memory, file count and
  total size before and after on the same 2-hour and 2-minute inputs
  ([quickstart.md](quickstart.md) §1).
- `task check` and `task check-docs` pass.

### Store and serve seek thumbnails as sprite sheets and crop them in the player

**Scope**:

- Change `internal/artifacts` to the location, `sprite.json`, completion check
  and old-format reclaiming of
  [R-3](research.md#r-3-storage-location-and-completion-marker), and make
  `Ingest.SeekThumbnails` in `internal/app` derive the layout from the video
  length and pass it to the previous unit's generation function. Remove the
  now-unused individual-JPEG generation in `internal/media`
  (`GenerateSeekThumbnails` and `Assets.SeekThumbnails`).
- Change `api/openapi.yaml` per
  [contracts/seek-sprite-api.md](contracts/seek-sprite-api.md), run
  `task generate`, and fix the layout information and sheet responses in
  `internal/httpapi` and the table in `auth.go`.
- Per [R-5](research.md#r-5-player-fetching-and-frame-cropping), change
  `web/src/player/seekPreview.ts` to fetch the layout information once, fetch
  each needed sheet once and hold it as an object URL, and crop frames by the
  size in the layout information. Drop the 5-second buckets and the
  one-at-a-time fetching of `fetchSeekThumbnail`, and align the fetch functions
  in `web/src/api/client.ts` with layout information and sheets.
  `VideoPlayer.tsx` remounts when `seekThumbnailState` changes. The display's
  look (size, position, time display) does not change.
- Only this unit changes `web/e2e/playback.e2e.ts` (the waits and the seek
  preview test).
- Fix the generated-files and workers paragraphs of ARCHITECTURE.md.

The API and the player switch in the same unit: the current player cannot read
the new response, and splitting them would break the seek preview on the
feature branch in between.

**Dependencies**: `Seek thumbnail layout rule and sprite sheet generation`.

**Acceptance**:

- Tests in `internal/artifacts` check: a location without `sprite.json` is
  judged incomplete; publishing deletes old-format individual JPEGs and
  replaces them with sheets and `sprite.json`; an interruption leaves nothing
  in the temporary location; `RemoveContent` deletes them.
- Tests in `internal/httpapi` check: the layout information is returned in the
  contract's shape with `private, no-cache` and `ETag`, and `If-None-Match`
  gives 304; an out-of-range sheet number gives 404; generation in progress
  gives 409.
- Web unit tests check: the rule that picks frame and sheet from the position
  uses the interval from the layout information and does not assume 5 seconds;
  moving within one sheet causes no fetch; moving across sheets fetches the next
  sheet once; returning to a sheet that was left before its fetch finished
  causes no second fetch; when fetching the layout information or a sheet
  fails, the time display continues and a retry happens after 5 seconds;
  unmounting aborts in-flight fetches and releases the object URLs.
- The playback test in `task test-e2e` checks that the same frame appears at
  the same position for both direct delivery and live transcoding.
- The screen changes, so with the 2-hour input that draws the time and the
  portrait input ([quickstart.md](quickstart.md) §2), confirm at 360px, 768px
  and 1280px that the times of frames near the start, middle and end lie in
  their ranges, that no neighbouring frame shows even in portrait, and that the
  browser's network log shows no additional sheet requests for the same video
  while the pointer moves continuously over the seek bar; record the results in
  the PR. Confirm that assistive technology still does not read the preview.
- `task check` and `task check-docs` pass.

### Rebuild existing seek thumbnails as sprites and align the tech stack and 009 docs

**Scope**: Per
[research.md R-6](research.md#r-6-migration-from-existing-individual-jpegs),
migration `00015_seek_thumbnail_sprite.sql` resets videos with
`seek_thumbnail_state = done` to `pending`, and queues a `seek_thumbnail` job
for each probed video with a location that has no unfinished one. Align the
seek preview description in `docs/design-docs/tech-stack-selection.md` with
sprite sheets, and replace the 5-second interval, individual JPEG and
`positionMs` descriptions in `plan.md`, `research.md`,
`contracts/seek-thumbnail.md` and `quickstart.md` of
`specs/009-seek-thumbnail-preview` with references to this feature's contract
(requirement 10).

**Dependencies**: `Store and serve seek thumbnails as sprite sheets and crop
them in the player`.

**Acceptance**: Tests in `internal/store` check that `00015` resets `done`
videos to `pending` and queues `seek_thumbnail`, does not queue for `failed`
videos or videos with an unfinished job, and that `up` / `down` pass
(`task migrations-check`). Restarting `task preview` with a data directory
holding the pre-change individual JPEGs shows remaining seek thumbnails in the
processing status; when they finish, only the sheets and `sprite.json` remain
in `seek/<p>/<s>/`, and in the meantime playback, seeking and the time display
work on the playback screen ([quickstart.md](quickstart.md) §3). `task check`
and `task check-docs` pass.
