# Contract: Live transcode quality

Source of truth: `api/openapi.yaml`. This document records only the parameter added to
`transcodeVideo` and the per-quality transcode guarantees. The response shape (fragmented MP4,
`Cache-Control: no-store`), the `transcode-start` report and the error shape
([specs/023-english-i18n/contracts/error-api.md](../../023-english-i18n/contracts/error-api.md)) do not
change.

## 1. `quality` on `GET /api/videos/{id}/transcode.mp4`

```yaml
- name: quality
  in: query
  required: false
  description: |
    Quality to scale down to. Without it, original quality (as before: the video stream is copied when it can be).
    With it, the video is always encoded, the display's short side is scaled to this value, and the bitrate is capped.
    Only qualities smaller than the video's display short side (the smaller of `Video.width` and `height`) are accepted.
  schema:
    type: string
    enum: [1080p, 720p, 480p, 360p]
```

| Case | Result |
| --- | --- |
| A value not in the enum | 400 `invalid_request`. |
| A quality at or above the video's short side, or a video without dimensions | 400 `invalid_request` ([research.md R-3](../research.md#r-3-availability-of-a-quality-depends-on-the-videos-short-side-and-the-server-rejects-unavailable-qualities-with-400)). |
| Combined with `startMs` and `attempt` | Works as before. A transcode with `quality` starts exactly at `startMs`, so the `transcode-start` report equals `startMs`. |
| Access | Stays "guests too" (requirement 8 of the parent Issue). |

## 2. Per-quality transcode guarantees

| `quality` | Display short side | Video cap (`-maxrate`) | `-bufsize` | Audio (AAC) |
| --- | --- | --- | --- | --- |
| `1080p` | 1080 | 5000 kbps | 10000 kbps | 128 kbps |
| `720p` | 720 | 2500 kbps | 5000 kbps | 128 kbps |
| `480p` | 480 | 1200 kbps | 2400 kbps | 96 kbps |
| `360p` | 360 | 700 kbps | 1400 kbps | 64 kbps |

- Dimensions are decided in display orientation (rotation applied): the aspect ratio is kept, the
  short side becomes the table value, and width and height are both rounded to even numbers. `720p`
  of a portrait 1080×1920 is 720×1280.
- Scaled dimensions do not exceed the current transcode frame (long side 3840, short side 2160). An
  extremely elongated video whose long side would exceed 3840 at the quality's short side is scaled
  further until it fits, so its short side is smaller than the table value (`1080p` of 1200×12000 is
  384×3840). The table's video cap and audio still apply.
- How each encoder applies the cap is in
  [research.md R-2](../research.md#r-2-quality-scales-the-short-side-of-the-display-and--maxrate-bufsize-caps-the-bitrate).
  H.264 High, Level 5.1, 4:2:0 8-bit, and a keyframe interval of 2 seconds or less in output time do
  not change.
- With a quality, audio is always encoded at the table's kbps (`-ac 2 -ar 48000`) and never copied.
- When a hardware method is unavailable and the transcode falls back to software, the same short side
  and caps apply.
