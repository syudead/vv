# Research: Reading the segments of the animated preview

Inherited decisions: the tech stack and the ownership of generated files follow
[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)
and [ARCHITECTURE.md](../../ARCHITECTURE.md) ("Generated files have one owner").
This file records only the decisions this feature adds.

The measurements come from two sources: those in parent Issue #387 (Windows,
and Linux with ffmpeg 6.1.1, 4 cores, 16GB), and runs in the development
container (Linux, ffmpeg 6.1.1, 4 cores, 16GB) on inputs made from `testsrc2`
(2 minutes, 1280×720, 30fps, H.264/AAC; 30 minutes, 640×360, 30fps, H.264;
`-g 250`). Both are synthetic video and do not guarantee the speed for videos
in a real library.

## R-1: How segments are read

**Decision**: For a video longer than 9 seconds, each of the 12 segments that
`PreviewSegments` returns is passed to a single ffmpeg as its own input,
`-ss <start> -t 0.75 -i <input>`. Each input gets
`[i:V:0]setpts=PTS-STARTPTS,<scale>,format=yuv420p[vi]`, and the inputs are
joined with `concat=n=12:v=1:a=0`. The output arguments
(`-c:v libx264 -pix_fmt yuv420p -movflags +faststart -an`) stay as they are.

**Rationale**: With `-ss` on the input side, ffmpeg seeks through the
container's index to the keyframe before the segment and decodes only from
there to the end of the segment. It holds the frames of one segment at a time,
so neither time nor memory depends on the length of the video. Measurements in
the development container:

| Input | Current (1 input, `trim`) | 12 inputs, 1 process | 12 processes, concat demuxer |
| --- | ---: | ---: | ---: |
| 2 minutes, 720p | 13.2–26.9 s, peak 5,010MB | 2.9–3.2 s, peak 488MB | 3.7–3.8 s, peak 126MB |
| 30 minutes, 640×360 | Reached 13.7GB at 57 s and was OOM-killed | 1.5–1.6 s, peak 270MB | 2.5 s, peak 106MB |

The parent Issue's 2-hour input shows the same direction (Linux: the current
method is OOM-killed; 12 inputs in 1 process takes 2.1–7.1 seconds and 367MB).
The single-process option is chosen because it starts one process (in the
parent Issue's short-video measurement on Windows, the option that starts 13
processes took 1.5 seconds against 0.6 seconds for the current method) and
because `internal/media` then needs no place for intermediate files. Its peak
memory is higher than the 12-process option, but it is set by the number of
segments and the resolution and does not grow with the video's length
(requirement 2).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Extract the 12 segments in separate processes and join them with the concat demuxer and `-c copy` | Rejected: smallest memory (above), but 13 process starts and intermediate files |
| Keep the old method for short videos only | Rejected: requirement 1 covers every video over 9 seconds, and even 2 minutes at 720p uses 5GB |
| Round segment starts to keyframes with `-noaccurate_seek` | Rejected: see R-3 |
| Put `-ss` on the output side per segment | Rejected: decodes the input from the start, the same cost as today |

## R-2: Segments with no frames

**Decision**: An input where a segment has no frames (for example, the
container is longer than the video stream and the last segment lies after the
end of the video) is not a failure; a shorter output that lacks that segment is
published. When all 12 segments are empty and the output is an empty file, the
existing check in `internal/artifacts.PublishPreview` (never publish an empty
file) fails it, and the job's retry and failure record handle it.

**Rationale**: Confirmed with an input of 10 seconds of video and 12 seconds of
audio (container length 12 seconds). The `concat` filter in ffmpeg 6.1.1 skips
an input that ended without producing frames as an empty segment, and produced
an output of 10 segments (7.67 seconds). The current method produces a
7.47-second output from the same input, so the behaviour does not change. The
`probe` length is the container's, and it is common for it to be a few hundred
milliseconds longer than the video stream; treating this as a failure would
leave many videos without a preview.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Move the last segment's start earlier to fit the video stream's length | Rejected: adds a way to pass the video stream's length into `internal/media`, and segment choice would vary by input |
| Detect empty segments and rerun ffmpeg | Rejected: unnecessary, ffmpeg skips them itself |

## R-3: Seek accuracy

**Decision**: `-ss` goes on the input side and uses ffmpeg's default (accurate
seek: decode from the preceding keyframe and drop frames up to the requested
time). `-noaccurate_seek` is not added.

**Rationale**: Today's `trim=start=<s>` emits frames from the requested time,
and that fixes the correspondence between segments and the scenes shown
(requirement 3, acceptance criterion 3). Accurate seek keeps the same
correspondence. Decoding the dropped frames costs one keyframe interval (at
most 8.3 seconds at 30fps with x264's default of 250 frames), and even for 12
segments together this is less than decoding the whole video (it is included
in the R-1 measurements).

**Alternatives considered**: `-noaccurate_seek` was rejected: segment starts
shift back to the preceding keyframe, and the interpretation of `-t` changes so
segments also get longer. It saves a little time but violates requirement 3.

## R-4: Measurement method

**Decision**: Write `scripts/previewbench` in Go. For each video given as an
argument, it runs `media.GeneratePreview` into an output in a temporary
directory `-runs` times (default 2) and prints the wall-clock time and
ffmpeg's peak memory for each run.

- Peak memory comes from `Maxrss` of `syscall.Getrusage(RUSAGE_CHILDREN)` on
  Linux/macOS and is printed per input as the maximum over all runs. The value
  is the maximum over all terminated child processes, so when several inputs
  are passed to one run, an input inherits the previous input's value; the
  how-to runs each input separately.
- On Windows, peak memory is not taken; only wall-clock time is printed.

The procedure lives in `docs/how-to/preview-benchmark.md`. It contains:

- the ffmpeg commands that create the comparison inputs from `testsrc2`
  (2 hours, 640×360, 30fps, H.264; 2 minutes, 1280×720, H.264/AAC; and a
  portrait input with rotation metadata for checking hover);
- the steps to measure the same inputs at the commit before the change and
  after it;
- the table format for recording the results in the PR;
- the steps to put the created inputs in `.local/preview/media/`, restart
  `task preview`, and check hover (the built-in samples of `task preview` are
  20 seconds or shorter, and it ingests that folder at every start).

**Rationale**: Requirement 6 asks for "a procedure that measures before and
after on the same input and environment", and that only means something if it
measures the production generation code itself (`GeneratePreview`). Written in
Go, the same procedure measures wall-clock time on Windows too. The parent
Issue's OOM happened on Linux, so measuring peak memory on Linux is enough to
show acceptance criterion 1. It also matches the repository rule (Taskfile.yml:
involved processing goes into a Go program under scripts/).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Only a shell and `/usr/bin/time -v` procedure in the how-to | Rejected: unusable on Windows, and copying the ffmpeg arguments into the how-to drifts from the production code |
| `go test -bench` | Rejected: cannot report the peak memory of child processes |
| Add a measurement hook to `GeneratePreview` | Rejected: leaves an argument in production code that exists only for measurement |
