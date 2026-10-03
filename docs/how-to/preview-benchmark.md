# Measure motion preview and seek thumbnail generation

In a PR that changes how motion previews (hover playback on library cards) or
seek thumbnails (the preview over the player's seek bar) are generated, measure
wall-clock time and peak memory before and after the change, with the same
inputs on the same environment, and record the results in the PR. The
measurement runs the production generation code itself, called by
`scripts/previewbench`: `GeneratePreview` in `internal/media` for motion
previews and `GenerateSeekThumbnailSet` for seek thumbnails. The reasoning is in
[specs/019-preview-input-seek/research.md R-4](../../specs/019-preview-input-seek/research.md#r-4-measurement-method).

## Prerequisites

- Go and `ffmpeg` (with `ffprobe`).
- A checkout of the PR branch.

## Steps

### 1. Create the inputs

Generate the inputs from `ffmpeg`'s `testsrc2`. They can live outside the
repository or under `.local/` (which git ignores); this guide uses
`.local/bench/`.

```sh
mkdir -p .local/bench

# 2 hours, 640x360, 30 fps, H.264 (long input; takes a few minutes)
ffmpeg -nostdin -v error -f lavfi -i testsrc2=size=640x360:rate=30:duration=7200 \
  -c:v libx264 -preset ultrafast -pix_fmt yuv420p .local/bench/long-2h.mp4

# 2 minutes, 1280x720, H.264/AAC (short input)
ffmpeg -nostdin -v error -f lavfi -i testsrc2=size=1280x720:rate=30:duration=120 \
  -f lavfi -i sine=frequency=440:duration=120 \
  -c:v libx264 -preset ultrafast -pix_fmt yuv420p -c:a aac -shortest .local/bench/short-2m-720p.mp4

# For the hover check: a portrait input with rotation metadata (90 degrees, 1 minute)
ffmpeg -nostdin -v error -f lavfi -i testsrc2=size=640x360:rate=30:duration=60 \
  -c:v libx264 -preset ultrafast -pix_fmt yuv420p .local/bench/landscape.mp4
ffmpeg -nostdin -v error -display_rotation 90 -i .local/bench/landscape.mp4 \
  -c copy .local/bench/portrait-rotated.mp4
```

The rotation metadata is present when `ffprobe .local/bench/portrait-rotated.mp4`
prints `rotation of 90.00 degrees` (displaymatrix).

### 2. Measure

```sh
go run ./scripts/previewbench [-kind preview|seek] [-runs N] <video>
```

`-kind` selects what to generate: `preview` (the default) for motion previews,
`seek` for seek thumbnails. For each input the tool runs the generation `-runs`
times (default 2) and prints the behaviour below.

| Output | Shown on |
| --- | --- |
| Wall-clock time of each run | All OSes |
| ffmpeg peak memory (the maximum over all runs) | Linux and macOS; Windows shows wall-clock time only |
| File count and total bytes in the output directory, per run | `-kind seek` only |

The tool deletes the generated output after each run and leaves no temporary
directory behind. A missing input or an ffmpeg failure prints the reason and
exits with a non-zero code.

Peak memory is the maximum over all finished child processes, so **run each
input separately**. When one run gets several inputs, a later input carries the
earlier input's value.

Measure before and after back to back on the same environment. To measure a
commit without `scripts/previewbench` (the "before" side), copy the "after"
program into a worktree and run it there.

```sh
# After (the PR branch)
go run ./scripts/previewbench .local/bench/long-2h.mp4
go run ./scripts/previewbench .local/bench/short-2m-720p.mp4

# Before (the PR's base commit)
bench="$PWD/.local/bench"
git worktree add ../vv-before <base commit>
cp -r scripts/previewbench ../vv-before/scripts/   # only when the base lacks it
(cd ../vv-before && go run ./scripts/previewbench "$bench/long-2h.mp4")
(cd ../vv-before && go run ./scripts/previewbench "$bench/short-2m-720p.mp4")
git worktree remove --force ../vv-before
```

Times vary with the disk cache, so report the first and second of the `-runs`
separately.

### 3. Record the results in the PR

Tabulate the environment (OS, CPU, ffmpeg version) and the values per input:
the wall-clock time of each run and the peak memory. PR bodies are Japanese, so
the template is in Japanese.

```markdown
環境: Linux x86_64, <CPU>, ffmpeg <版>

| 入力 | 変更前 1 回目 | 変更前 2 回目 | 変更前ピーク | 変更後 1 回目 | 変更後 2 回目 | 変更後ピーク |
| --- | --- | --- | --- | --- | --- | --- |
| 2 時間・640×360・H.264 | 00.000 秒 | 00.000 秒 | 0.0 MiB | 00.000 秒 | 00.000 秒 | 0.0 MiB |
| 2 分・1280×720・H.264/AAC | 00.000 秒 | 00.000 秒 | 0.0 MiB | 00.000 秒 | 00.000 秒 | 0.0 MiB |
```

When the "before" side failed (for example, it ran out of memory), write that
and the printed reason in the cell. On Windows, write `測らない` (not measured)
in the peak columns.

#### Seek thumbnails

With `-kind seek`, measure the same 2-hour `long-2h.mp4` and 2-minute
`short-2m-720p.mp4`, one run per input. The file count and total cover
everything `GenerateSeekThumbnailSet` wrote to the output directory. Report the
total in MiB.

```sh
go run ./scripts/previewbench -kind seek .local/bench/long-2h.mp4
go run ./scripts/previewbench -kind seek .local/bench/short-2m-720p.mp4
```

```markdown
環境: Linux x86_64, <CPU>, ffmpeg <版>

| 入力 | 変更前 1 回目 | 変更前 2 回目 | 変更前ピークメモリ | 変更前ファイル数 | 変更前合計 | 変更後 1 回目 | 変更後 2 回目 | 変更後ピークメモリ | 変更後ファイル数 | 変更後合計 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 2 時間・640×360・H.264 | 00.000 秒 | 00.000 秒 | 0 MiB | 0 | 0.0 MiB | 00.000 秒 | 00.000 秒 | 0 MiB | 0 | 0.0 MiB |
| 2 分・1280×720・H.264/AAC | 00.000 秒 | 00.000 秒 | 0 MiB | 0 | 0.0 MiB | 00.000 秒 | 00.000 秒 | 0 MiB | 0 | 0.0 MiB |
```

On an OS where peak memory is not available (such as Windows), write `取得不可`
(not available) in the peak memory columns.

### 4. Measure long HD input-side seeking

When comparing approaches for long HD videos, use the following 1080p inputs in
addition to the 360p input above. They repeat a 30-second source by stream
copy, so the 2 hours need no re-encoding. The content is a repeated synthetic
source and does not reproduce real videos or NAS I/O performance.

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

Frame boundaries during the copy can make the length slightly exceed 7200
seconds. Do not pass a hand-computed 7200 seconds; use the actual length the
benchmark reads from ffprobe. For the conditions that select an approach and the
behaviour when frames are missing, see the
[design document](../design-docs/seek-sprite-generation.md).

## Check the result on hover

The built-in samples of `task preview` are 20 seconds or shorter, so do not use
them to check long inputs. Put the generated inputs in `.local/preview/media/`
and restart `task preview`, which scans that folder on every start
([Check a PR in Codespaces](codespaces-preview.md)).

```sh
mkdir -p .local/preview/media
cp .local/bench/long-2h.mp4 .local/bench/portrait-rotated.mp4 .local/preview/media/
task preview   # if it is running, stop it with Ctrl+C first, then start it again
```

After preview generation finishes, hover over the library cards and check:

| Input | Expected |
| --- | --- |
| 2-hour input | Scenes spread from the start to the end (visible in the `testsrc2` clock) play in chronological order for about 9 seconds, silently, in a loop. |
| Portrait input | It plays in portrait without a distorted aspect ratio. |
