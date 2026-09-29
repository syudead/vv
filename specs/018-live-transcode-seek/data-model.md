# Data model: probe data for live transcoding

This document covers only what is stored for Requirements 7 to 10 of parent Issue #371, and its
rules. Existing tables (`videos`, `video_locations`, `jobs` and others) do not change. The table
classification follows "Rebuildable and user data" in [ARCHITECTURE.md](../../ARCHITECTURE.md); this
table is an index.

## 1. Migration

Add `internal/store/migrations/00013_transcode_probes.sql`.

```sql
-- Probe data that live transcoding needs. It is an index; when lost, the next transcode or re-probe refills it (Requirement 10).
create table video_transcode_probes (
    video_id   integer primary key references videos (id) on delete cascade,
    -- domain.TranscodeProbeVersion. A different value reads as "absent" (§3).
    version    integer not null,
    -- Size and modification time (Unix nanoseconds) from os.Stat of the probed file. Compared with the file opened at request time (§4).
    size_bytes integer not null,
    mtime_ns   integer not null,
    -- JSON of domain.TranscodeProbe (§2).
    probe      text    not null,
    updated_at integer not null
) without rowid;
```

A row is deleted by cascade when its `videos` row is deleted. The same happens when a video's content
changes and its row is recreated; the new row is filled through the path of Requirement 10.

## 2. Stored value: `domain.TranscodeProbe`

A value type in `internal/domain`. It rides on `domain.Probe` as `Transcode *TranscodeProbe`, and the
ingest probe (`app.Ingest.Probe`) passes it to the store unchanged. For a video with no usable video
stream (not attached, has dimensions), it is nil and no row is written (the video cannot be live
transcoded).

| Field | Content | Used for |
| --- | --- | --- |
| `FormatName` | `format.format_name` (lowercase) | Deciding on MOV dual input |
| `Video.Index` | `index` of the chosen video stream (the first non-attached stream) | `-map` |
| `Video.CodecName`, `Profile`, `Level`, `PixelFormat`, `BitsPerRawSample` | Video encoding | `videoCanCopy` |
| `Video.Width`, `Height`, `SampleAspectNum`, `SampleAspectDen`, `Rotation` | Geometry (rotation from the Display Matrix or the `rotate` tag) | Dimensions, aspect ratio, rotation handling |
| `Video.FPS`, `RealFPS` | `avg_frame_rate`, `r_frame_rate` | Variable frame rate detection and the fps cap |
| `Audio` (nullable) | `Index`, `CodecName`, `Profile`, `SampleRate`, `Channels` of the chosen audio stream (the first stream) | `-map`, `audioCanCopy` |

The fields match the current `transcodeMetadata`
([internal/media/transcode.go](../../internal/media/transcode.go)); the type moves to
`internal/domain`. JSON keys use the Go field names as is. Adding a field or changing a meaning bumps
`domain.TranscodeProbeVersion`.

## 3. Usability check: `domain.TranscodeProbeUsable`

A pure function. The stored value is used only when all of these hold:

1. The row exists.
2. `version` equals the current `domain.TranscodeProbeVersion`. The Edge case "the ffprobe output
   format changes" is handled by the version, because the stored shape is the parser's value.
3. `probe` parses as a `TranscodeProbe`.
4. `size_bytes` and `mtime_ns` equal the `Stat` of the file the transcode actually opened
   (Requirement 9). For another location with the same content, the comparison uses the values of
   the opened location.

Otherwise the value is treated as absent: ffprobe runs at request time and its result replaces the
row (Requirements 9 and 10).

## 4. When it is written and read

| When | Operation | Identity |
| --- | --- | --- |
| The ingest probe job (`ApplyProbeForJob`) and `ApplyProbe` | Upsert in the same transaction as the `videos` update | The `os.Stat` that `media.Probe` takes just before ffprobe |
| Re-probe (`POST /api/videos/{id}/probe`) | Same as above (through the job) | Same as above |
| Live transcoding runs ffprobe on the spot (`SaveTranscodeProbe`) | 1 upsert statement | `Stat` of the file the transcode opened |
| Live transcode start (`LibraryStore.TranscodeProbe`) | Read 1 row | Compared as in §3 |

- The upsert is 1 statement (`insert … on conflict (video_id) do update`). When ingest and transcode
  write at the same time, no corrupt value remains and the later write wins (Edge case "multiple
  requests at the same time").
- An interrupted ffprobe ends with an error, so nothing is saved (Edge case "when the request is
  cancelled midway").
- Nothing bulk-fills existing videos at startup or at ingest (Requirement 10).
