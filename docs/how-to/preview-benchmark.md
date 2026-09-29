# Measuring animated preview and seek thumbnail generation

Use this procedure in a PR that changes how animated previews (hover playback on list cards) or
seek thumbnails (the seek bar preview on the playback screen) are generated. It measures wall
clock time and peak memory before and after the change, on the same input and in the same
environment, and records them in the PR.

`scripts/previewbench` calls the production generation code itself: `GeneratePreview` in
`internal/media` for animated previews, and `GenerateSeekThumbnailSet` for seek thumbnails. The
reasons are in
[specs/019-preview-input-seek/research.md R-4](../../specs/019-preview-input-seek/research.md#r-4-measurement-method).

## Create the inputs

Generate the inputs from `ffmpeg` `testsrc2`. They can live outside the repository or under
`.local/` (git ignores `.local/`). This page uses `.local/bench/`.

```sh
mkdir -p .local/bench

# 2 h, 640x360, 30 fps, H.264 (long input; takes a few minutes)
ffmpeg -nostdin -v error -f lavfi -i testsrc2=size=640x360:rate=30:duration=7200 \
  -c:v libx264 -preset ultrafast -pix_fmt yuv420p .local/bench/long-2h.mp4

# 2 min, 1280x720, H.264/AAC (short input)
ffmpeg -nostdin -v error -f lavfi -i testsrc2=size=1280x720:rate=30:duration=120 \
  -f lavfi -i sine=frequency=440:duration=120 \
  -c:v libx264 -preset ultrafast -pix_fmt yuv420p -c:a aac -shortest .local/bench/short-2m-720p.mp4

# For the hover check: a portrait input with rotation metadata (90 degrees), 1 min
ffmpeg -nostdin -v error -f lavfi -i testsrc2=size=640x360:rate=30:duration=60 \
  -c:v libx264 -preset ultrafast -pix_fmt yuv420p .local/bench/landscape.mp4
ffmpeg -nostdin -v error -display_rotation 90 -i .local/bench/landscape.mp4 \
  -c copy .local/bench/portrait-rotated.mp4
```

The rotation metadata is present when the output of `ffprobe .local/bench/portrait-rotated.mp4`
shows `rotation of 90.00 degrees` (displaymatrix).

## Measure

```sh
go run ./scripts/previewbench [-kind preview|seek] [-runs N] <video>
```

| Flag or case | Behavior |
| --- | --- |
| `-kind` | The generation to measure: `preview` (default) for animated previews, `seek` for seek thumbnails. |
| `-runs` | Runs the generation N times per input (default 2). |
| Output, every kind | Wall clock time per run. On Linux and macOS, also the ffmpeg peak memory (maximum over all runs). On Windows, wall clock time only. |
| Output, `-kind seek` | Also the file count and total bytes of the output directory per run. |
| Cleanup | The generated output is deleted after every run; no temporary directory remains after exit. |
| Missing input or ffmpeg failure | Prints the reason and exits with a non-zero code. |

Peak memory is the maximum over all terminated child processes, so **run each input
separately**. When one run gets several inputs, a later input keeps the value of an earlier one.

Measure before and after back to back in the same environment. To measure a commit without
`scripts/previewbench` (before the change), copy the program from the changed branch into a
worktree and run it there.

```sh
# After the change (the PR branch)
go run ./scripts/previewbench .local/bench/long-2h.mp4
go run ./scripts/previewbench .local/bench/short-2m-720p.mp4

# Before the change (the PR base commit)
bench="$PWD/.local/bench"
git worktree add ../vv-before <base commit>
cp -r scripts/previewbench ../vv-before/scripts/   # only when base lacks it
(cd ../vv-before && go run ./scripts/previewbench "$bench/long-2h.mp4")
(cd ../vv-before && go run ./scripts/previewbench "$bench/short-2m-720p.mp4")
git worktree remove --force ../vv-before
```

Disk caching changes the time, so report the first and second `-runs` run separately.

## Format for the PR

Record the environment (OS, CPU, ffmpeg version) and a table of values per input: the wall clock
time of each run and the peak memory.

```markdown
Environment: Linux x86_64, <CPU>, ffmpeg <version>

| Input | Before run 1 | Before run 2 | Before peak | After run 1 | After run 2 | After peak |
| --- | --- | --- | --- | --- | --- | --- |
| 2 h, 640×360, H.264 | 00.000 s | 00.000 s | 0.0 MiB | 00.000 s | 00.000 s | 0.0 MiB |
| 2 min, 1280×720, H.264/AAC | 00.000 s | 00.000 s | 0.0 MiB | 00.000 s | 00.000 s | 0.0 MiB |
```

When the before run failed (for example, it crashed out of memory), write that and the printed
reason in the table cell. On Windows, write `not measured` in the peak columns.

### Seek thumbnails

Measure the same 2 h `long-2h.mp4` and 2 min `short-2m-720p.mp4` with `-kind seek`, one run per
input. The file count and total cover everything `GenerateSeekThumbnailSet` wrote to the output
directory. Report the total in MiB.

```sh
go run ./scripts/previewbench -kind seek .local/bench/long-2h.mp4
go run ./scripts/previewbench -kind seek .local/bench/short-2m-720p.mp4
```

```markdown
Environment: Linux x86_64, <CPU>, ffmpeg <version>

| Input | Before run 1 | Before run 2 | Before peak memory | Before file count | Before total | After run 1 | After run 2 | After peak memory | After file count | After total |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 2 h, 640×360, H.264 | 00.000 s | 00.000 s | 0 MiB | 0 | 0.0 MiB | 00.000 s | 00.000 s | 0 MiB | 0 | 0.0 MiB |
| 2 min, 1280×720, H.264/AAC | 00.000 s | 00.000 s | 0 MiB | 0 | 0.0 MiB | 00.000 s | 00.000 s | 0 MiB | 0 | 0.0 MiB |
```

On an OS without peak memory (such as Windows), write `unavailable` in the peak memory
columns.

## Check on hover

The built-in `task preview` samples are 20 s or shorter, so do not use them for the long input
check. Put the generated inputs in `.local/preview/media/` and restart `task preview`. Every
`task preview` start imports that folder ([Checking a PR in Codespaces](codespaces-preview.md)).

```sh
mkdir -p .local/preview/media
cp .local/bench/long-2h.mp4 .local/bench/portrait-rotated.mp4 .local/preview/media/
task preview   # if it is running, stop it with Ctrl+C first, then restart
```

After preview generation finishes, hover over the list cards and check:

- The 2 h input: scenes spread from the start to the end (visible in the `testsrc2` clock) play in
  chronological order for about 9 s, muted, in a loop.
- The portrait input: it plays in portrait, and the aspect ratio is kept.

### Input-side seek on long HD inputs

To compare methods for long HD inputs, use the following 1080p inputs in addition to the 360p
input above. They repeat a 30 s seed by stream copy, so 2 h need not be re-encoded. The content is
a repeated synthetic seed; it does not reproduce real videos or NAS I/O performance.

```sh
ffmpeg -nostdin -v error -f lavfi -i testsrc2=size=1920x1080:rate=30:duration=30 \
  -c:v libx264 -preset veryfast -crf 28 -g 75 -keyint_min 75 -sc_threshold 0 \
  -pix_fmt yuv420p .local/bench/h264-hd-seed.mp4
ffmpeg -nostdin -v error -stream_loop -1 -i .local/bench/h264-hd-seed.mp4 \
  -t 7200 -c copy .local/bench/h264-hd-2h.mp4

ffmpeg -nostdin -v error -f lavfi -i testsrc2=size=1920x1080:rate=30:duration=30 \
  -c:v libx265 -preset veryfast -crf 28 -g 75 \
  -x265-params keyint=75:min-keyint=75:scenecut=0:pools=8 \
  -pix_fmt yuv420p10le .local/bench/hevc-hd-seed.mp4
ffmpeg -nostdin -v error -stream_loop -1 -i .local/bench/hevc-hd-seed.mp4 \
  -t 7200 -c copy .local/bench/hevc-hd-2h.mp4

go run ./scripts/previewbench -kind seek .local/bench/h264-hd-2h.mp4
go run ./scripts/previewbench -kind seek .local/bench/hevc-hd-2h.mp4
```

Frame boundaries during the copy can make the duration slightly longer than 7200 s. Do not pass a
hand-computed 7200 s; use the actual duration the benchmark gets from ffprobe. The method
selection conditions and the behavior on missing output are in the
[design document](../design-docs/seek-sprite-generation.md).
