# Validation Quickstart: seek thumbnail sprite sheets

The common checks are in [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`,
`task test-e2e`). This file lists only the inputs and steps specific to this feature. As in
[docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md), the inputs are
generated from `testsrc2` (which draws the time on screen) and stored in `.local/bench/`. Do not
use private videos or file names.

## 1. Generation time, file count and total size before and after (Acceptance criteria 1 and 2)

Use the 2 h input `long-2h.mp4` and the 2 min input `short-2m-720p.mp4` from
`docs/how-to/preview-benchmark.md`. Run `go run ./scripts/previewbench -kind seek <video>` both
before and after the change. Measure "before" on the commit before generation changes (right after
previewbench gains the seek kind).

Put the environment (OS, CPU, ffmpeg version) and the table below in the PR. The file count and
total cover everything in the previewbench output directory (what `GenerateSeekThumbnailSet`
writes). After the change it holds only sheets: 6 for 2 h and 1 for 2 min (`sprite.json` is not
included, because the storage side writes it).

```markdown
| Input | Before run 1 | Before run 2 | Before peak memory | Before file count | Before total | After run 1 | After run 2 | After peak memory | After file count | After total |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 2 h, 640×360, H.264 | 00.000 s | 00.000 s | 0 MiB | 1440 | 0.0 MiB | 00.000 s | 00.000 s | 0 MiB | 6 | 0.0 MiB |
| 2 min, 1280×720, H.264/AAC | 00.000 s | 00.000 s | 0 MiB | 24 | 0.0 MiB | 00.000 s | 00.000 s | 0 MiB | 1 | 0.0 MiB |
```

On an OS without peak memory, write `unavailable`. The `internal/domain` tests check
that the 2 min input gets a 5 s interval and the 2 h input a 12 s interval with 600 frames.

## 2. Checks on the playback screen (Acceptance criteria 3–6)

Create extra inputs: the portrait input is `portrait-rotated.mp4` from `preview-benchmark.md`, and
the live transcoding input is the same content in MKV (a container the browser cannot play).

```sh
ffmpeg -nostdin -v error -i .local/bench/long-2h.mp4 -c copy .local/bench/long-2h.mkv
mkdir -p .local/preview/media
cp .local/bench/long-2h.mp4 .local/bench/long-2h.mkv .local/bench/portrait-rotated.mp4 .local/preview/media/
task preview   # if it is running, stop it with Ctrl+C first, then restart
```

After the remaining seek thumbnail count in the processing status reaches 0, check on each
video's playback screen.

| Case | Action | Expected evidence |
| --- | --- | --- |
| Start, middle, end (3) | On the 2 h MP4, point at the start, center and near the end of the seek bar | The time drawn in the shown frame lies in that position's interval `[k*12s, (k+1)*12s)` |
| Tracking (4) | Open the network log in the browser developer tools and move the pointer continuously across the seek bar from end to end | Only 1 layout request and 1 request per sheet passed; no repeated request for the same sheet |
| Portrait (5) | Point on `portrait-rotated.mp4` | The frame stays portrait inside the box, and no part of the neighboring frame shows |
| Live transcoding (6) | Point at the same position (for example the center) on `long-2h.mkv` and `long-2h.mp4` | Both show the frame for the same time |
| Short video | The built-in `task preview` samples (20 s or shorter) | 1 sheet with a few frames; no black frame even at the end |

At 360 px, 768 px and 1280 px, capture the start, center and end and add them to the PR. Also
compare that the preview's size, position and time label match the state before the change.

## 3. Interruption, replacement, deletion and cleanup of the old JPEGs (Acceptance criteria 7 and 8)

- Stopping `task preview` with Ctrl+C during generation (while the processing status shows
  remaining seek thumbnails) and restarting it empties `MDM_DATA_DIR/thumbnails/.tmp/`. No
  directory without `sprite.json` remains in that video's location.
- Replacing an input in `.local/preview/media/` with a same-named file of different content during
  generation and rescanning never serves the old content's sprite
  (`GET /api/videos/{id}/seek-thumbnail` returns 409 and then 200 with the new version). The
  sprite is rebuilt from the new content.
- Restarting with a data directory from before the change (individual JPEGs
  `seek/<p>/<s>/000000.jpg` …) shows remaining seek thumbnails in the processing status. When
  they finish, only the sheets and `sprite.json` remain in that location. While work remains,
  playback, seeking and the time display still work on that video's playback screen.
