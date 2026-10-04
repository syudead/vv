# Validation Quickstart: Seek thumbnail sprite sheets

These steps cover the inputs and checks specific to this feature; the common
checks are in [Taskfile.yml](../../Taskfile.yml) (`task check`,
`task check-docs`, `task test-e2e`). Inputs are made from `testsrc2` (which
draws the time on screen), as in
[docs/how-to/preview-benchmark.md](../../docs/how-to/preview-benchmark.md), and
placed in `.local/bench/`. Do not use private videos or file names.

## Generation time, file count and total size before and after (acceptance criteria 1 and 2)

Using the 2-hour input `long-2h.mp4` and the 2-minute input
`short-2m-720p.mp4` from `docs/how-to/preview-benchmark.md`, run
`go run ./scripts/previewbench -kind seek <video>` both before and after the
change. Measure "before" at the commit before generation changed (right after
the seek kind was added to previewbench).

Put the environment (OS, CPU, ffmpeg version) and the following table in the
PR. File count and total cover everything in previewbench's output directory
(what `GenerateSeekThumbnailSet` writes). After the change there are only
sheets: 6 for 2 hours and 1 for 2 minutes (`sprite.json` is written by the
storage side and is not included).

```markdown
| 入力 | 変更前 1 回目 | 変更前 2 回目 | 変更前ピークメモリ | 変更前ファイル数 | 変更前合計 | 変更後 1 回目 | 変更後 2 回目 | 変更後ピークメモリ | 変更後ファイル数 | 変更後合計 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 2 時間・640×360・H.264 | 00.000 秒 | 00.000 秒 | 0 MiB | 1440 | 0.0 MiB | 00.000 秒 | 00.000 秒 | 0 MiB | 6 | 0.0 MiB |
| 2 分・1280×720・H.264/AAC | 00.000 秒 | 00.000 秒 | 0 MiB | 24 | 0.0 MiB | 00.000 秒 | 00.000 秒 | 0 MiB | 1 | 0.0 MiB |
```

On an OS where peak memory cannot be taken, write `取得不可`. Tests in
`internal/domain` check that the 2-minute input gets a 5-second interval and
the 2-hour input a 12-second interval with 600 frames.

## Checks on the playback screen (acceptance criteria 3–6)

Create the additional inputs: for portrait, `portrait-rotated.mp4` from
`preview-benchmark.md`; for live transcoding, the same content as MKV (a
container the browser cannot play).

```sh
ffmpeg -nostdin -v error -i .local/bench/long-2h.mp4 -c copy .local/bench/long-2h.mkv
mkdir -p .local/preview/media
cp .local/bench/long-2h.mp4 .local/bench/long-2h.mkv .local/bench/portrait-rotated.mp4 .local/preview/media/
task preview   # if it is running, stop it with Ctrl+C and start it again
```

After the remaining seek thumbnail count in the processing status reaches 0,
check on each video's playback screen.

| Case | Action | Expected evidence |
| --- | --- | --- |
| Start, middle, end (3) | On the 2-hour MP4, point at the start, centre, and near the end of the seek bar | The time drawn in the shown frame lies within the range `[k*12s, (k+1)*12s)` for that position |
| Following (4) | Open the network log in the browser's developer tools and move the pointer continuously across the seek bar from end to end | Only one request for the layout information and one per sheet passed over; no repeated request for the same sheet |
| Portrait (5) | Point on `portrait-rotated.mp4` | The frame stays portrait inside the box, and no part of a neighbouring frame shows |
| Live transcode (6) | Point at the same position (for example the centre) on `long-2h.mkv` and `long-2h.mp4` | The frame for the same time appears |
| Short video | The built-in samples of `task preview` (20 seconds or shorter) | One sheet with a few frames; no black frame even at the end |

At each of 360px, 768px and 1280px, capture the display at the start, centre
and end and put the captures in the PR. Also compare that the preview's size,
position and time display are the same as before the change.

## Interruption, replacement, deletion, and reclaiming existing JPEGs (acceptance criteria 7 and 8)

- Stopping `task preview` with Ctrl+C during generation (while the processing
  status shows remaining seek thumbnails) and starting it again leaves
  `MDM_DATA_DIR/thumbnails/.tmp/` empty, and no directory without `sprite.json`
  remains in that video's location.
- Replacing an input in `.local/preview/media/` during generation with a file
  of the same name and different content and rescanning: the old content's
  sprite is not served (`GET /api/videos/{id}/seek-thumbnail` returns 409 for
  the new version, then 200), and it is rebuilt with the new content.
- Restarting with a data directory from before the change (individual JPEGs
  `seek/<p>/<s>/000000.jpg` …) shows remaining seek thumbnails in the processing
  status. When they finish, only the sheets and `sprite.json` remain in that
  location. While work remains, playback, seeking and the time display work on
  that video's playback screen.
