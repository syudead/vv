# Media test fixtures

These synthetic inputs replace repeated FFmpeg setup in media tests.
The recipes are in [generate.go](generate.go); no downloaded media is included.

## Test boundaries

| Input | Test boundary |
| --- | --- |
| `fingerprints/*.jpg` | `SpriteFingerprint` decodes, crops, hashes and compares stored sheets without FFmpeg |
| `videos/*` | Media adapters still run FFmpeg on these inputs and inspect newly generated output |

Fingerprint tests no longer cover the combined video-to-sprite-to-fingerprint
pipeline or the installed encoder's output variations. Sprite generation has
its own adapter tests; those checks do not claim the removed combined boundary.
Fixture readers share immutable files, while generated test outputs use `t.TempDir()`.

## Fingerprint sheets

Each sheet has a 9 by 9 grid, with unused cells black.
The source scene changes every 20 seconds and varies by the source identifier.

| Sheet | Source | Frames | Interval (ms) | Frame size | Variation |
| --- | --- | --- | --- | --- | --- |
| `short.jpg` | A, 60 s | 60 | 1000 | 320 by 180 | H.264 CRF 20 |
| `letterbox.jpg` | A, 60 s | 60 | 1000 | 320 by 240 | H.264 CRF 32 with black bands |
| `other.jpg` | B, 60 s | 60 | 1000 | 320 by 180 | Different content at the same duration |
| `long.jpg` | A, 410 s | 81 | 5062 | 320 by 180 | H.264 CRF 20 |
| `long-reencoded.jpg` | A, 410.4 s | 81 | 5067 | 320 by 180 | H.264 CRF 30 and a different GOP |
| `long-other.jpg` | B, 410 s | 81 | 5062 | 320 by 180 | Different content at the same duration |
| `old-size.jpg` | A, 60 s | 60 | 1000 | 160 by 90 | JPEG quality 4 instead of 2 |

## Video inputs

| Files | Property retained from the runtime-generated input |
| --- | --- |
| `clock.mp4`, `frame-clock.mp4`, `short-clock.mp4` | Luma encodes time, for position and end-of-input checks |
| `short-video.mp4`, `longer-container.mp4` | Audio extends beyond the video |
| `sparse.mp4` | Video frames at 0, 10 and 20 seconds |
| `one-second.mp4`, `green.mp4` | Short constant-colour inputs |
| `portrait.mp4`, `large.mp4` | Portrait aspect and the 2048-pixel frame cap |
| `preview.mp4`, `preview-short.mp4` | H.264 input for preview encoding and cancellation |
| `rotated.mp4` | A 1280 by 720 video with a 90-degree display rotation |
| `with-cover.mp4`, `timeline-with-cover.mp4` | Attached pictures that extraction must ignore |
| `sparse-keyframes.ts` | MPEG-TS without a seekable keyframe after the beginning |

## Regeneration

Run these commands from the repository root with Go and FFmpeg on `PATH`.
The committed files were generated with FFmpeg 7.1.5 (libx264 enabled).

1. Run `go run ./internal/media/testdata/generate.go` to replace the fixtures and `SHA256SUMS`.
2. Run `git diff -- internal/media/testdata` to review changes; FFmpeg versions can change the bytes.
3. Run `go test -count=1 ./internal/media` to verify the regenerated inputs against the assertions.

The generator builds intermediate videos in a temporary directory and removes
it on success; normal tests never invoke it or regenerate missing fixtures.
