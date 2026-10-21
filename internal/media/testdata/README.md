# Media test fixtures

These synthetic inputs replace repeated FFmpeg setup in media tests.
The recipes are in [generate.go](generate.go); no downloaded media is included.

## Test boundaries

| Input | Test boundary |
| --- | --- |
| `fingerprints/*.mp4` | `VideoFingerprint` runs FFmpeg on versions an owner would bundle and compares their fingerprints |
| `videos/*` | Media adapters still run FFmpeg on these inputs and inspect newly generated output |

Fixture readers share immutable files, while generated test outputs use `t.TempDir()`.

## Fingerprint videos

The source scene changes every 20 seconds and drifts continuously, so a start
shifted by one second no longer lines up frame for frame.

| Video | Source | Variation |
| --- | --- | --- |
| `base.mp4` | A, 61 s | 320 by 180, H.264 CRF 20 |
| `letterbox.mp4` | A, 61 s | 240 by 180 with black bands, CRF 32 |
| `watermark.mp4` | A, 61 s | A semi-transparent white box over the top right |
| `shifted.mp4` | A from 1 s | Starts one second later, CRF 30 |
| `other.mp4` | B, 61 s | Different content at the same duration |

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
The committed files were generated with FFmpeg 7.1.5 (libx264 enabled); `fingerprints/` with FFmpeg 6.1.1.

1. Run `go run ./internal/media/testdata/generate.go` to replace the fixtures and `SHA256SUMS`.
2. Run `git diff -- internal/media/testdata` to review changes; FFmpeg versions can change the bytes.
3. Run `go test -count=1 ./internal/media` to verify the regenerated inputs against the assertions.

The generator builds intermediate videos in a temporary directory and removes
it on success; normal tests never invoke it or regenerate missing fixtures.
