# Research: reading motion preview segments

The tech stack and the ownership of generated files follow their sources of truth
([docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
[ARCHITECTURE.md](../../ARCHITECTURE.md) "Generated files have one owner"). This document records
only the decisions this feature adds.

Measurements come from two sources:

- Parent Issue #387 (Windows, and Linux with ffmpeg 6.1.1, 4 cores, 16 GB).
- The dev container (Linux, ffmpeg 6.1.1, 4 cores, 16 GB), with inputs made from `testsrc2`:
  2 min, 1280×720, 30 fps, H.264/AAC; and 30 min, 640×360, 30 fps, H.264, `-g 250`.

Both use synthetic video and do not guarantee speed on real library videos.

## R-1: Segment reading method

| | |
| --- | --- |
| **Decision** | For a video longer than 9 s, pass each of the 12 segments that `PreviewSegments` returns to one ffmpeg run as an input `-ss <start> -t 0.75 -i <input>`. Apply `[i:V:0]setpts=PTS-STARTPTS,<scale>,format=yuv420p[vi]` per input and join with `concat=n=12:v=1:a=0`. The output arguments (`-c:v libx264 -pix_fmt yuv420p -movflags +faststart -an`) stay as they are. |
| **Why** | With input-side `-ss`, ffmpeg seeks through the container index to the keyframe before the segment and decodes only up to the segment end. It keeps only one segment's frames, so time and memory do not depend on video length (measurements below). One process is chosen because it starts ffmpeg once (in the parent Issue's short Windows measurement, 13 launches took 1.5 s against 0.6 s for the current method) and `internal/media` needs no place for intermediate files. Peak memory is higher than with 12 processes, but the segment count and resolution decide it, not the video length (Requirement 2). |
| **Rejected** | Extracting the 12 segments in separate processes and joining with the concat demuxer and `-c copy` (above; minimum memory, but 13 launches and intermediate files). Keeping the old method for short videos only (Requirement 1 covers every video over 9 s, and 2 min at 720p still uses 5 GB). Rounding segment starts to keyframes with `-noaccurate_seek` (R-3). Placing `-ss` on the output side per segment (decodes the input from the start, so the cost equals today's). |

Dev container measurements:

| Input | Current (1 input, `trim`) | 12 inputs, 1 process | 12 processes, concat demuxer |
| --- | ---: | ---: | ---: |
| 2 min, 720p | 13.2–26.9 s, peak 5,010 MB | 2.9–3.2 s, peak 488 MB | 3.7–3.8 s, peak 126 MB |
| 30 min, 640×360 | Reaches 13.7 GB at 57 s, OOM kill | 1.5–1.6 s, peak 270 MB | 2.5 s, peak 106 MB |

The parent Issue's 2 h input shows the same trend (Linux: the current method hits OOM; 12 inputs
in 1 process take 2.1–7.1 s and 367 MB).

## R-2: Segments without frames

| | |
| --- | --- |
| **Decision** | An input with a segment that has no frames (for example, the container is longer than the video stream and the last segment starts after the video ends) does not fail. The preview is published shorter, without that segment. When all 12 segments are empty and the output is an empty file, the existing check in `internal/artifacts.PublishPreview` (never publish an empty file) fails it, and job retry and failure recording handle it. |
| **Why** | Verified with an input of 10 s video and 12 s audio (container length 12 s). The ffmpeg 6.1.1 `concat` filter skipped an input that ended without frames as an empty segment and produced 10 segments of output (7.67 s). The current method produces 7.47 s from the same input, so the behavior does not change. The `probe` duration is the container's, and it is often a few hundred ms longer than the video stream; failing on this would block previews for many videos. |
| **Rejected** | Shifting the last segment start earlier to fit the video stream length: adds a way to pass the video stream length into `internal/media`, and the segment selection would vary by input. Detecting empty segments and rerunning ffmpeg: unnecessary, because ffmpeg skips them itself. |

## R-3: Seek accuracy

| | |
| --- | --- |
| **Decision** | Place `-ss` on the input side and keep the ffmpeg default (accurate seek: decode from the previous keyframe and discard frames before the requested time). Do not add `-noaccurate_seek`. |
| **Why** | Today's `trim=start=<s>` outputs frames from the requested time, which fixes the mapping between segments and the scenes shown (Requirement 3, Acceptance criterion 3). Accurate seek keeps the same mapping. The decode cost of discarded frames is at most one keyframe interval (with the x264 default of 250 frames at 30 fps, up to 8.3 s). For all 12 segments together it is smaller than decoding the whole video (included in the R-1 measurements). |
| **Rejected** | `-noaccurate_seek`: segment starts shift to the previous keyframe, and the interpretation of `-t` changes so segments get longer. It is slightly faster but violates Requirement 3. |

## R-4: Measurement method

| | |
| --- | --- |
| **Decision** | Write `scripts/previewbench` in Go. For each video argument, it runs `media.GeneratePreview` `-runs` times (default 2) into a temporary directory and prints the wall-clock time per run and the ffmpeg peak memory. The procedure lives in `docs/how-to/preview-benchmark.md`. Details are below. |
| **Why** | Requirement 6 asks for "a procedure that measures before and after with the same input and environment", and the measurement is meaningful only on the production generation code itself (`GeneratePreview`). Go lets Windows measure wall-clock time with the same procedure. The parent Issue's OOM happens on Linux, so measuring peak memory on Linux is enough to show Acceptance criterion 1. It also fits the repository rule (Taskfile.yml: put complex processing in a Go program under scripts/). |
| **Rejected** | Only shell and `/usr/bin/time -v` steps in the how-to: unusable on Windows, and copying the ffmpeg arguments into the doc would drift from the production code. `go test -bench`: cannot report child process peak memory. Adding a measurement hook to `GeneratePreview`: leaves an argument in production code that exists only for measurement. |

Details of `scripts/previewbench`:

- Peak memory comes from `Maxrss` of `syscall.Getrusage(RUSAGE_CHILDREN)` on Linux and macOS,
  printed per input as the maximum across all runs.
- This value is the maximum over all finished child processes, so passing several inputs to one
  run carries over the previous input's value. The how-to runs each input separately.
- Windows does not collect it and prints only wall-clock time.

Content of `docs/how-to/preview-benchmark.md`:

- ffmpeg commands that make comparison inputs from `testsrc2`: 2 h, 640×360, 30 fps, H.264; and
  2 min, 1280×720, H.264/AAC. A portrait input with rotation metadata is also made for the hover
  check.
- Steps that measure the same inputs on the commit before the change and after it.
- The form for recording the results as a table in the PR.
- Steps that put the inputs in `.local/preview/media/`, restart `task preview`, and check them by
  hover. The built-in `task preview` samples are 20 s or shorter, and every start imports that
  folder.
