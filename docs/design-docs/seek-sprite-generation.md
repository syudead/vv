# Seek sprite generation

A newly generated seek preview picks up to 81 frames from the whole video and lays them out in one
9×9 sprite, each frame at most 160px on its long side.

- Short videos keep a 5-second interval. Longer videos place a frame every
  `ceil(durationMs / 81)` milliseconds.
- Frame k covers the interval `[k × intervalMs, (k + 1) × intervalMs)`.
- The player picks a frame for a playback position from `intervalMs`, `frameCount`, `columns` and
  `rows` in the layout data, so the player's calculation needs no change.

## Choosing frames

Before generation starts, the image for each interval is decided. An interval with no image is
common: when the container is slightly longer than the video, the last interval starts where the
video ends, and some videos have keyframe intervals longer than the sprite interval. Such an
interval is not a failure; it uses the scene of the previous interval. Only the first interval,
which has no previous one, uses the scene of the next interval. The scene of a later interval is
never moved forward.

## Generating from the index (H.264/HEVC in MP4/MOV)

When the first video track of an MP4/MOV is H.264 or HEVC, the `moov` sample table is read once.
It gives each keyframe's presentation time (computed from `stts`, `ctts` and the edit list) and its
position in the file.

- Each interval uses the first keyframe inside it.
- When the interval has none, it uses the last keyframe before the interval.
- When the edit list sets the end of the playback range, keyframes after it are not candidates.

Only the bytes of the selected keyframes are read, with up to 8 reads in parallel. Each is written
to a temporary directory, one file per frame, in start-code-delimited form with the parameter sets
from the decoder configuration (`avcC`/`hvcC`) prepended. Intervals that share a keyframe read and
decode it once, and the image is duplicated.

Decoding runs in one FFmpeg invocation and produces 160px images.

- When every selected keyframe is IDR, they are joined into one stream and decoded by one decoder.
  An IDR resets the display order numbering and the decoder state.
- When non-IDR keyframes are mixed in (CRA in an open GOP, or non-IDR I-frames), one stream would
  continue the numbering from the previous keyframe, and the decoder would reorder frames across
  distant keyframes. Each frame is therefore a separate input with its own decoder, joined in input
  order with the `concat` filter. This is slower by the per-input initialization.
- When the number of decoded images does not match, the result is not used, because the frames
  would be misaligned.

Decoding raw video loses the rotation in the display matrix of `tkhd`. When the display matrix is a
rotation in 90-degree steps, the same transform that ffmpeg's autorotation applies when reading
from a container (`transpose`, `hflip,vflip`) is applied before scaling. Other display matrices,
such as flips, are out of scope.

Reading the index and the keyframes returns on the job deadline or a stop without waiting for the
read to finish. This keeps a job from hanging on a storage location that does not respond. When
everything is ready, FFmpeg tiles the images 9×9 into `000.jpg`.

Each frame is the first keyframe in its interval, not the exact start of the interval. In most
videos keyframes are a few seconds apart, so the scene difference is small. This approach neither
launches FFmpeg per frame to reread the index nor decodes from a keyframe to the target time. Reads
per video are limited to the index and the keyframes (tens to hundreds of KB per frame).

These inputs use the per-interval extraction below:

- no `moov` (such as fragmented MP4)
- video that is not H.264/HEVC
- a container that is not MP4/MOV
- a resolution or similar change midway (more than one sample description)
- an edit that cuts out a middle part (more than one non-empty edit)
- a display matrix with anything other than rotation

When reading or decoding fails, for example because the index is broken, a warning is logged and
generation switches to per-interval extraction.

## Per-interval extraction

For inputs that cannot use the index, FFmpeg runs once per frame. It seeks to the start of the
interval on the input side and writes a BMP to a temporary directory.

- Up to 4 run at once. The frame number is the file name, so the layout does not depend on the
  order in which extractions finish.
- Each extraction stops at the end of its interval. When an interval has no frame, the adjacent
  frame is duplicated by the rule above.
- For inputs with several video streams, the first video stream that is not an attached picture is
  used.
- The temporary BMPs are deleted on success, failure and cancellation alike. Publishing the job
  still happens after `internal/artifacts` verifies the finished output, as before.

Only when FFmpeg exits with an error, or no interval yields an image, does generation fall back to
full decoding with the same frame count, 160px and 9×9 layout. A cancellation midway does not start
full decoding. The 30-minute processing limit applies to extraction, tiling and the fallback
together.

Full decoding is reported in the log and in the return value of `GenerateSeekSprite`. It is
recorded as a recent scan issue (substitute, `seek_thumbnail_full_decode`)
([specs/024-import-progress/research.md](../../specs/024-import-progress/research.md) R-7). An
input that cannot use the index but succeeds with per-interval extraction takes the normal path and
does not count as a substitute.

Whether full decoding was used is also kept in `fullDecode` of `sprite.json`. A rerun after a stop
between publishing and recording completion reads it back and records the substitute. So does
another video with the same content that adopts the finished sprite. An older `sprite.json`
without `fullDecode` does not say whether a substitute was used, so the issue row is left
unchanged.

Older finished sprites (up to 600 frames, 6 sheets) remain readable. There is no bulk
regeneration to introduce the new method; it applies to newly processed jobs. This keeps the
existing queue from growing.

## Speed and limits

Measured on a 21-minute 1080p H.264 video on a local disk (81 frames, all keyframes IDR):

| Method | Time | Data read |
| --- | --- | --- |
| Per-interval extraction (4 parallel) | 5.6–5.9 s | Hundreds of MB per video (each frame rereads the index and the frames from the keyframe to the target time) |
| Index, one decoder | 0.37 s | About 15 MB (index about 4 MB plus 81 keyframes) |
| Index, the same 81 frames as separate inputs | 1.6–1.8 s (videos with non-IDR keyframes take about this long) | — |

On storage with slow random reads, the time depends on the number of reads (one for the index plus
one per keyframe) and the latency of each. Generation from the index reads less and is faster than
per-interval extraction, but one read per frame remains. Measurements check both the content of
the result and the time taken.
