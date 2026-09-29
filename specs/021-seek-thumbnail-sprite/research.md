# Research: seek thumbnails as sprite sheets with caps on frame and file counts

> [!NOTE]
> The generation method was later updated by
> [seek sprite generation for long videos](../../docs/design-docs/seek-sprite-generation.md).
> The full decode described below is the initial implementation's decision. It now applies only
> to inputs that neither the index nor per-interval extraction can handle.

The tech stack, boundaries, ownership of generated files, and the `seek_thumbnail` job and state
are inherited unchanged from the existing sources of truth ([ARCHITECTURE.md](../../ARCHITECTURE.md),
[specs/020-seek-thumbnail-stage/data-model.md](../020-seek-thumbnail-stage/data-model.md)). This
file records only the decisions this feature adds.

## R-1: At most 600 frames on 6 sheets; the interval widens only past 50 min

| | |
| --- | --- |
| **Decision** | A video's sprite has at most 6 sheets of 10 columns × 10 rows (100 frames per sheet), and at most 600 frames. Pure functions in `internal/domain` derive the interval, frame count and sheet count from the video duration `durationMs`. |
| **Why** | Requirement 2 says "keep 5 s within the cap, and widen only for longer videos", so the cap sets the length that 5 s can cover. 600 frames (50 min) keeps the current interval for videos under 1 h, the majority of the library, and still lets a 2 h film be searched at 12 s intervals (1/600 of the seek bar). |
| **Rejected** | A cap of 1,440 frames (5 s up to 2 h). A 25-frame sheet (5 × 5) with a higher sheet cap. Smaller frames for more frames per sheet. Reasons below. |

```text
intervalMs = max(5000, ceil(durationMs / 600))
frameCount = max(1, ceil(durationMs / intervalMs))      # never above 600
sheetCount = ceil(frameCount / 100)                      # never above 6
positions covered by frame k: [k * intervalMs, (k + 1) * intervalMs)
frame for position p:         min(floor(p / intervalMs), frameCount - 1)
```

Up to 50 min (3,000,000 ms), the interval stays at the current 5 s. Only longer videos get a
wider interval.

| Duration | Interval | Frames | Sheets |
| --- | --- | --- | --- |
| 1 h | 6 s | 600 | 6 |
| 2 h | 12 s | 600 | 6 |
| 3 min | 5 s | 36 | 1 (the last 64 frame slots are empty) |

- The frame size stays as today: the frame fits a 320 × 320 box in its displayed orientation,
  rounded to even numbers
  (`scale=min(320,iw):min(320,ih):force_original_aspect_ratio=decrease:force_divisible_by=2`).
  All frames of one video have the same size.
- A 100-frame sheet (3200 × 1800 px at 16:9, about 0.7 MB per sheet for the testsrc2 input) fits
  what a browser decodes as one image. 6 sheets turn today's 1,440 files of about 12 MB into 6
  files of a few MB.

Rejected alternatives:

- A cap of 1,440 frames (5 s up to 2 h): a 2 h film gets 15 sheets of tens of MB, which defeats
  the goal of "a few sheets".
- 25 frames per sheet (5 × 5) with a higher sheet cap: each fetch is lighter, but moving across
  the bar of a 2 h video takes 24 fetches, narrowing the range where Requirement 5 "switches
  without fetching again" holds.
- Smaller frames to fit more per sheet: changing how the preview looks is out of scope.

## R-2: Frames come from `fps`, `tpad`, `trim` and `tile` in one decode

| | |
| --- | --- |
| **Decision** | Generation stays one full decode, as today. Instead of `select`, the `fps` filter takes 1 frame per interval, `tpad` clones the last frame, `trim` cuts the count to `frameCount`, and `tile` lays the frames out in a grid. |
| **Why** | `fps` produces evenly spaced output and fills intervals without a frame by cloning the previous frame, so frame numbers never shift. `tpad` and `trim` fix `frameCount` to the value derived from the duration, so the result need not be recounted. |
| **Rejected** | Adding `tile` to the current `select` expression. Input-side seeking 600 times, as for animated previews. Cutting sheets with `-frames:v`. Reasons below. |

The bundled ffmpeg 6.1 runs the following form. Verified with a 30 s input with 23 s of video and
30 s of audio: the 6th frame showed the 23 s scene, and one sheet held 6 frames. Inputs of 0.2 s,
1 s and 2.6 s each produced one sheet with 1 frame.

```text
ffmpeg -nostdin -v error -i <video> -map 0:V:0? \
  -vf "fps=1000/<intervalMs>:eof_action=pass,tpad=stop_mode=clone:stop=-1,trim=end_frame=<frameCount>,\
scale=min(320\,iw):min(320\,ih):force_original_aspect_ratio=decrease:force_divisible_by=2,\
tile=10x10,format=yuvj420p" \
  -fps_mode passthrough -q:v 4 -start_number 0 -y <tmp>/%03d.jpg
```

- With the default `fps` rounding (`round=near`), frame k shows roughly the scene at
  `k * interval + interval / 2`: the middle of the interval it covers. The first frame shows the
  scene at half the interval, not at 0 s. This meets Requirement 4 ("the shown scene matches the
  playback position") at the middle of each interval.
- `eof_action=pass` emits the last not-yet-emitted frame as one frame at the end of input. With
  the default, an input shorter than half the interval (2.5 s at a 5 s interval) makes `fps` emit
  no frame, `tpad` has nothing to clone, and no sheet is produced (verified with a 1 s input).
- With `pass`, the last scene of such an input becomes the first frame, which lies inside the
  interval the first frame covers (the whole video). For inputs of at least half the interval,
  the number and times of the frames are the same as with the default.

Details of the reasoning:

- The current `select` takes "the first frame of the interval", but it skips intervals without a
  frame (variable frame rate or a broken index). Every later number then shifts by one, and
  position and scene no longer match (Edge cases).
- When the video is shorter than the container duration (no frames near the end), `fps` alone
  drops the last frames and `tile` fills them with black. `tpad` clones the last frame and `trim`
  cuts at `frameCount` to prevent that.
- `tile` writes a sheet when it has one sheet of frames or when input ends, and fills the empty
  part of the last sheet with black. The player never points beyond `frameCount`, so the empty
  part is never visible.

Rejected alternatives:

- Adding `tile` to the current `select` expression: the shift above remains.
- Reading 600 times with input-side seeking, as animated previews do (specs/019): a 2 h video
  needs 600 seeks and decodes, which is not necessarily faster than a full decode (84 s at
  640 × 360). The requirement caps frame and file counts, not the generation method, so this
  feature does not change it. The PR records generation time before and after (Acceptance
  criterion 1).
- Cutting sheets with `-frames:v`: after `tile`, the count is the sheet count and cannot cut
  frames, so `trim` goes before `tile`.

## R-3: The location stays `seek/<p>/<s>/`; `sprite.json` marks completion

| | |
| --- | --- |
| **Decision** | The location stays `seek/<p>/<s>/`. It holds sheets `000.jpg`–`005.jpg` (3-digit numbers, at most 6) and the layout file `sprite.json`. `internal/artifacts` writes `sprite.json` after generation and gets the frame size by dividing the JPEG dimensions of sheet `000.jpg` by the column and row counts. The presence of `sprite.json` marks completion. |
| **Why** | The layout describes the generated files, so storing and renaming it together with them keeps them consistent (the same shape as the `.sha256` manifest of hover previews). Keeping the directory lets existing deletion and cleanup work unchanged. |
| **Rejected** | A column in `videos`. A new `sprite/` directory with bulk deletion of `seek/` at startup. No layout file, with Go and TypeScript constants. Reasons below. |

```json
{"version": 1, "intervalMs": 12000, "frameCount": 600, "columns": 10, "rows": 10,
 "frameWidth": 320, "frameHeight": 180, "sheetCount": 6}
```

- `SeekThumbnailsAvailable` and serving check `sprite.json`. A location without `sprite.json` (old
  individual JPEGs, or a broken partial write) counts as incomplete.
- The existing `Catalog.SeekThumbnailState` rule (a `done` video without a location returns to
  `pending` and is queued once) queues the rebuild unchanged.
- Publishing works as today: write everything under `.tmp`, then rename the whole directory. An
  old-format location is deleted just before the rename, inside the per-content lock.
- The frame size is read from the finished sheet, not computed from the ffmpeg expression or the
  probe values. This avoids reproducing ffmpeg autorotation and even rounding, and the structure
  guarantees that the generating side and the display side use the same numbers (Requirement 3).
- Keeping the directory means `RemoveContent`, startup cleanup and `ContentUnreferenced` deletion
  work unchanged, and the old format is collected in the same place.

Rejected alternatives:

- Storing the layout in a `videos` column: the column is updated separately from the generated
  files, so restoring only the data directory makes them disagree. `internal/store` does not look
  at the file system, so it cannot check.
- A new `sprite/` directory, deleting the old `seek/` in bulk at startup: startup cleanup would
  walk the whole store, breaking the ARCHITECTURE.md rule "clean nothing except `.tmp`".
- No layout file, with the route rules (columns, rows, interval) as Go and TypeScript constants:
  frame size and actual frame count differ per video, so constants are not enough and
  Requirement 3 ("use the same information") cannot be met.

## R-4: The API returns a JSON layout and separate JPEG sheets

| | |
| --- | --- |
| **Decision** | `GET /api/videos/{id}/seek-thumbnail` (the versioned URL `Video.seekThumbnailUrl` points to) takes no `positionMs` and returns the layout as JSON. `GET /api/videos/{id}/seek-thumbnail/{sheet}` returns a sheet as JPEG. The layout lists the versioned sheet URLs. Both responses use `private, no-cache` and `ETag` and also serve guests. Details: [contracts/seek-sprite-api.md](contracts/seek-sprite-api.md). |
| **Why** | `Video.seekThumbnailUrl` and `seekThumbnailState` have the shape fixed in 020 and appear in list responses. Embedding the layout in `Video` would read `sprite.json` on every list response, so the layout is a separate response the playback screen fetches once. One route returning JSON or JPEG depending on `sheet` would give one OpenAPI operation two response types, so the routes are separate. |
| **Rejected** | Renaming the route to `seek-sprite`: jobs, states and URLs are all named `seek_thumbnail` / `seekThumbnail`, and renaming only the route obscures the mapping. Keeping a `positionMs` response for the old SPA: the SPA ships inside the binary and updates with it, and no earlier contract change kept the old form either. |

## R-5: The player fetches each sheet once and crops it with the background

| | |
| --- | --- |
| **Decision** | The player fetches the layout when it first shows the preview, picks the frame and sheet with the [R-1](#r-1-at-most-600-frames-on-6-sheets-the-interval-widens-only-past-50-min) rule, fetches a missing sheet with `fetch`, and holds it as a `URL.createObjectURL` URL. The preview box shows one frame of the sheet as a background image. |
| **Why** | Requirement 5 and Acceptance criterion 4 require "no additional request for sheets of the same video", so the browser HTTP cache cannot be relied on. The current responses are `no-cache` (guest-api.md §5), and every reassignment of the same URL to `img.src` sends a revalidation request. An object URL needs only one request. |
| **Rejected** | Showing the whole sheet in an `<img>` with `object-fit` and `object-position`: the window position lands on fractional pixels, and after scaling 1 px of the neighboring frame can show. Fetching the layout when the playback screen appears: a video still generating returns 409, and it breaks the current rule "request only on hover", so the layout is fetched when the preview first shows. |

Fetching:

- Sheets the player holds stay for the whole mount (at most 6) and are released on unmount.
- At most one fetch runs per sheet; in-flight fetches are also tracked by sheet number.
- Moving within a sheet fetches nothing. Moving across sheets leaves the previous sheet's
  in-flight fetch running and starts a fetch for the next sheet if it is not held.
- Returning to a previous sheet with an in-flight fetch waits for that fetch and sends no new
  request.
- Only unmount aborts in-flight fetches. A finished fetch for a sheet not being shown does not
  switch the display.
- A failed layout or sheet fetch shows only the time, as today. A failed sheet is not fetched
  again for 5 s.

Cropping:

- The preview box is one frame's box. The sheet is the background image, tiled at `columns` times
  the width and `rows` times the height, and `background-position` shifts it by the matching
  column and row.
- The box aspect ratio comes from `frameWidth : frameHeight` in the layout (today it comes from
  the probe aspect ratio).
- The shift is an integer multiple of the box size, so scaling fractions never reveal the
  neighboring frame (Requirement 6).
- The preview size, position and time label do not change.

## R-6: A migration requeues existing individual JPEGs for rebuild

| | |
| --- | --- |
| **Decision** | Migration `00015_seek_thumbnail_sprite.sql` returns videos with `seek_thumbnail_state = done` to `pending`, and queues a `seek_thumbnail` job for each analyzed video with a location and no unfinished `seek_thumbnail` job. `failed` videos return through a read retry, as before. |
| **Why** | Requirement 9 ("becomes a generation target without a re-import instruction") is met by queueing in a migration, as 020's [00014_seek_thumbnail_stage.sql](../../internal/store/migrations/00014_seek_thumbnail_stage.sql) did. Relying only on the current rule that queues when the playback screen opens shows no remaining work in the processing status until then, so nobody knows when it finishes. |
| **Rejected** | Serving the old format until the rebuild finishes. Queueing no `jobs` in the migration and relying on the location check. Returning `failed` videos to `pending` too. Reasons below. |

- The migration does not touch the states and jobs of cover thumbnails and animated previews
  (Requirement 9).
- During the rebuild, the video's seek preview behaves like today's `pending` (time only).
  Playback, seeking and the time display work (Acceptance criterion 8).
- `internal/artifacts` deletes the old individual JPEGs when it publishes that video's sprite
  ([R-3](#r-3-the-location-stays-seek-spritejson-marks-completion)).
- Acceptance criterion 8 requires playback, seeking and the time display during the rebuild; the
  preview image is not included.

Rejected alternatives:

- Serving the old format until the rebuild finishes: the player and the API would keep two
  formats, which contradicts "replace" in Requirement 1.
- Queueing no `jobs` in the migration and relying only on the location check: the remaining
  work stays invisible, as explained above.
- Returning `failed` videos to `pending` too: the causes of failure (unreadable input, the 30 min
  cap) do not depend on the method, and the retry count record would be lost.
