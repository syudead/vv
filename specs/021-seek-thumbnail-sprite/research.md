# Research: Seek thumbnails as sprite sheets, with limits on frame and file count

> The generation method was later updated by
> [seek sprite generation for long videos](../../docs/design-docs/seek-sprite-generation.md).
> The full-video decode described below is the decision of the initial
> implementation; it is now used only for inputs that can be built neither from
> the index nor by per-segment extraction.

Inherited decisions: the tech stack, boundaries, ownership of generated files,
and the `seek_thumbnail` job and state are taken over unchanged from
[ARCHITECTURE.md](../../ARCHITECTURE.md) and
[specs/020-seek-thumbnail-stage/data-model.md](../020-seek-thumbnail-stage/data-model.md).
This file records only the decisions this feature adds.

## R-1: Limits and interval rule

**Decision**: A video's sprite has at most 6 sheets of 10 columns × 10 rows
(100 frames per sheet), and at most 600 frames. Pure functions in
`internal/domain` derive the interval, frame count and sheet count from the
video length `durationMs`.

```text
intervalMs = max(5000, ceil(durationMs / 600))
frameCount = max(1, ceil(durationMs / intervalMs))      # never exceeds 600
sheetCount = ceil(frameCount / 100)                      # never exceeds 6
positions covered by frame k: [k * intervalMs, (k + 1) * intervalMs)
frame for position p:         min(floor(p / intervalMs), frameCount - 1)
```

| Video length | Interval | Frames | Sheets |
| --- | --- | --- | --- |
| Up to 50 minutes (3,000,000 ms) | 5 seconds (unchanged) | Up to 600 | Up to 6 |
| 2 hours | 12 seconds | 600 | 6 |
| 1 hour | 6 seconds | 600 | 6 |
| 3 minutes | 5 seconds | 36 | 1 (the last 64 cells empty) |

Only videos longer than 50 minutes get a wider interval. The frame size is the
same as today: fitted, in display orientation, into a 320 × 320 box and rounded
to even numbers
(`scale=min(320,iw):min(320,ih):force_original_aspect_ratio=decrease:force_divisible_by=2`).
All frames of one video have the same size.

**Rationale**: Requirement 2 is "keep 5 seconds within the limit and widen only
for lengths beyond it", so the limit is the number that decides which lengths 5
seconds can cover. 600 frames (50 minutes) keeps the current interval for
videos under an hour, which make up most of a library, and still lets a 2-hour
film be searched at a 12-second interval (1/600 of the seek bar). 100 frames
per sheet (3200 × 1800 px at 16:9, around 0.7 MB per sheet for the testsrc2
input) stays within a size a browser can decode as one image, and 6 sheets turn
today's 1,440 files of about 12 MB into 6 files of a few MB.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Limit of 1,440 frames (5 seconds up to 2 hours) | Rejected: a 2-hour film gets 15 sheets and tens of MB, against the goal of "a few sheets" |
| 25 frames per sheet (5 × 5) with a higher sheet limit | Rejected: each fetch is lighter, but moving across a 2-hour bar makes 24 fetches, narrowing what requirement 5 ("switch without refetching") covers |
| Smaller frames to fit more per sheet | Rejected: changing how the preview looks is out of scope |

## R-2: Frame selection

**Decision**: Generation remains one full-video decode as today. Instead of
`select`, the `fps` filter takes one frame per interval, `tpad` clones the last
frame, `trim` cuts the frame count to `frameCount`, and `tile` lays the frames
out in a grid. The following was confirmed to work with the bundled ffmpeg 6.1
(on a 30-second input with 23 seconds of video and 30 seconds of audio, the
sixth frame showed the 23-second scene and one sheet held 6 frames; inputs of
0.2, 1 and 2.6 seconds each gave one sheet with one frame).

```text
ffmpeg -nostdin -v error -i <video> -map 0:V:0? \
  -vf "fps=1000/<intervalMs>:eof_action=pass,tpad=stop_mode=clone:stop=-1,trim=end_frame=<frameCount>,\
scale=min(320\,iw):min(320\,ih):force_original_aspect_ratio=decrease:force_divisible_by=2,\
tile=10x10,format=yuvj420p" \
  -fps_mode passthrough -q:v 4 -start_number 0 -y <tmp>/%03d.jpg
```

With the default rounding of `fps` (`round=near`), frame k shows the scene at
about `k * interval + interval / 2`, the middle of the range it covers. The
first frame is the scene at half an interval, not at 0 seconds (requirement 4,
"the scene shown corresponds to the playback position", is met at the middle of
the range).

`eof_action=pass` emits the last not-yet-emitted frame as one frame at the end
of the input. With the default `round`, an input shorter than half an interval
(2.5 seconds at a 5-second interval) makes `fps` emit no frame at all, `tpad`
has nothing to clone, and no sheet is produced (confirmed with a 1-second
input). With `pass`, the last scene of that input becomes the first frame,
which lies within the range the first frame covers (the whole video). For
inputs of at least half an interval, the number and times of frames are the
same as with `round`.

**Rationale**: Today's `select` takes "the first frame of each range", but it
skips ranges with no frames (variable frame rate or a broken index), so every
later number shifts by one and positions stop matching scenes (edge cases).
`fps` makes the output evenly spaced and clones the previous frame for a range
with no frames, so numbers do not shift. For an input whose video is shorter
than the container (no frames near the end), `fps` alone drops the trailing
frames and `tile` fills them with black, so `tpad` clones the last frame and
`trim` cuts at `frameCount`. The `frameCount` in the layout information is
then fixed by the value derived from the video length, and the generated result
does not need to be recounted. `tile` writes a sheet when one sheet's worth has
accumulated or the input ends, and fills the rest of the last sheet with black.
The player never points past `frameCount`, so the empty cells are never seen.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Keep today's `select` expression and add `tile` | Rejected: the shift above remains |
| Read 600 times with an input-side seek, as the animated preview does (specs/019) | Not changed in this feature: 600 seeks and decodes for 2 hours are not necessarily faster than the full decode (84 seconds at 640 × 360), and the requirement is a limit on frame and file counts, not the generation method. Generation time before and after is recorded in the PR (acceptance criterion 1) |
| Cut sheets with `-frames:v` | Rejected: after `tile` the count is the sheet count and cannot cut the frame count, so `trim` goes before `tile` |

## R-3: Storage location and completion marker

**Decision**: The location stays `seek/<p>/<s>/`, and its content becomes
sheets `000.jpg`–`005.jpg` (three-digit numbers, at most 6) plus the layout
information `sprite.json`. `internal/artifacts` writes `sprite.json` after
generation, and gets the frame size by dividing the JPEG dimensions of sheet
`000.jpg` by the column and row counts.

```json
{"version": 1, "intervalMs": 12000, "frameCount": 600, "columns": 10, "rows": 10,
 "frameWidth": 320, "frameHeight": 180, "sheetCount": 6}
```

The completion marker is the presence of `sprite.json`; `SeekThumbnailsAvailable`
and delivery check it. A location without `sprite.json` (old-format individual
JPEGs, or one broken midway) is treated as incomplete, and the existing rule of
`Catalog.SeekThumbnailState` (if `done` but the stored files are missing, reset
to `pending` and queue once) queues the rebuild as is. Publishing still writes
everything under `.tmp` and then renames the whole directory; if an old-format
location exists, it is deleted inside the per-content lock right before the
rename.

**Rationale**: The layout information describes the generated files, so
placing it with them and renaming them together keeps the two from disagreeing
(the same shape as the hover preview's `.sha256` manifest). The frame size is
read from the finished sheet rather than computed from ffmpeg expressions or
probe values, so ffmpeg's auto-rotation and even rounding need not be
reproduced, and the structure guarantees that the generating side and the
displaying side use the same numbers (requirement 3). The location directory
stays the same so that deletion by `RemoveContent`, the startup cleanup and
`ContentUnreferenced` keeps working, and old-format files are reclaimed in the
same place.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Keep the layout information in a `videos` column | Rejected: the column is updated separately from the generated files and disagrees when only the data directory is restored; `internal/store` does not look at the file system and cannot check |
| Store in a new `sprite/` directory and delete the old `seek/` in bulk at startup | Rejected: the startup cleanup would walk the whole storage, breaking the ARCHITECTURE.md rule "clean up nothing except `.tmp`" |
| No layout information; make the path rules (columns, rows, interval) Go and TypeScript constants | Rejected: frame size and actual frame count differ per video, so constants are not enough and requirement 3 ("use the same information") is not met |

## R-4: API shape

**Decision**: `GET /api/videos/{id}/seek-thumbnail` (the versioned URL that
`Video.seekThumbnailUrl` points to) no longer takes `positionMs` and returns
the layout information as JSON. `GET /api/videos/{id}/seek-thumbnail/{sheet}`
returns a sheet as JPEG. The layout information lists the sheets' versioned
URLs. Details: [contracts/seek-sprite-api.md](contracts/seek-sprite-api.md).
Both responses carry `private, no-cache` and `ETag`, and both are returned to
guests too.

**Rationale**: `Video.seekThumbnailUrl` and `seekThumbnailState` have the shape
decided in 020 and also appear in the list response. Embedding the layout
information in `Video` would read `sprite.json` for every list response, so it
is a separate response that the playback screen fetches once. One path that
returns JSON or JPEG depending on whether `sheet` is present would give one
OpenAPI operation two response types, so the paths are separate.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Rename the path to `seek-sprite` | Rejected: job, state and URL names line up as `seek_thumbnail` / `seekThumbnail`, and renaming only the path obscures the correspondence |
| Keep the `positionMs` response for an old SPA | Rejected: the SPA ships inside the binary and updates with it, and earlier contract changes kept nothing either |

## R-5: Player fetching and frame cropping

**Decision**: The player fetches the layout information when it first shows the
preview, picks the frame and sheet for the position by the rule in
[R-1](#r-1-limits-and-interval-rule), and, if it does not hold that sheet yet,
fetches it with `fetch` and keeps it as a `URL.createObjectURL` URL.

| Situation | Behaviour |
| --- | --- |
| Sheets held | Kept for the whole time the player is mounted (at most 6), released on unmount |
| Fetches per sheet | At most one; in-flight fetches are also tracked by sheet number |
| Moving within one sheet | No fetch |
| Moving across sheets | The previous sheet's in-flight fetch is left running, not aborted; a fetch starts for the next sheet if it is not held |
| Moving back to the previous sheet | If its fetch is in flight, wait for it and use it; no new request |
| Aborting in-flight fetches | Only on unmount |
| A fetch completes for a sheet not being shown | The display does not switch |
| Layout information or sheet fetch fails | Time-only display, as today; a failed sheet is not refetched for 5 seconds |

Frame cropping: the preview box is one frame's box; the sheet is its background
image, laid out at `columns` times the width and `rows` times the height, and
shifted by `background-position` to the right column and row. The box's aspect
ratio comes from `frameWidth : frameHeight` in the layout information (today it
comes from the probe's aspect ratio). Shifts are whole multiples of the box
size, so scaling fractions never reveal the neighbouring frame
(requirement 6). The preview's size, position and time display do not change.

**Rationale**: Requirement 5 and acceptance criterion 4 say "no additional
requests for the same video's sheets", so the browser's HTTP cache cannot be
relied on. Today's response is `no-cache` (guest-api.md, [Cache of generated files](../016-single-account-auth/contracts/guest-api.md#cache-of-generated-files)), and setting the
same URL on `img.src` again triggers a revalidation request each time. With an
object URL, one request is enough.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Put the whole sheet in an `<img>` and show it with `object-fit` and `object-position` | Rejected: the window's position lands on pixel fractions, and after scaling 1 px of the neighbouring frame can show |
| Fetch the layout information when the playback screen opens | Rejected: a video being generated returns 409, and it departs from today's rule "request only on hover"; it is fetched on first display |

## R-6: Migration from existing individual JPEGs

**Decision**: Migration `00015_seek_thumbnail_sprite.sql` resets videos with
`seek_thumbnail_state = done` to `pending`, and queues a `seek_thumbnail` job
for each probed video with a location that has no unfinished one. `failed`
videos are reset by a probe retry as before. The state and jobs of cover
thumbnails and animated previews are not touched (requirement 9). During the
rebuild, that video's seek preview is treated like today's `pending`
(time-only display), and playback, seeking and the time display remain usable
(acceptance criterion 8). `internal/artifacts` deletes the old-format
individual JPEGs when it publishes that video's sprite
([R-3](#r-3-storage-location-and-completion-marker)).

**Rationale**: Requirement 9, "becomes a generation target without a re-ingest
instruction", is met by queueing in the migration, as 020's migration
[00014_seek_thumbnail_stage.sql](../../internal/store/migrations/00014_seek_thumbnail_stage.sql)
does. Relying only on today's rule of queueing when the playback screen opens
would show no remaining work in the processing status until the screen is
opened, and nobody would know when it finishes. Serving the old format until
the rebuild finishes would keep two formats in the player and the API, against
requirement 1's "replace". What acceptance criterion 8 requires during the
rebuild is playback, seeking and the time display; the preview image is not
included.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Serve the old format until the rebuild finishes | Rejected: see above |
| Queue no `jobs` in the migration and rely on the stored-files check | Rejected: see above |
| Reset `failed` videos to `pending` too | Rejected: the causes of failure (unreadable input, the 30-minute limit) are unrelated to the method, and the record of retry counts would be lost |
