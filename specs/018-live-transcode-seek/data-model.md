# Data model: Probe data for live transcoding

This file covers only what parent Issue #371 requirements 7–10 store and the
rules for it. The existing tables (`videos`, `video_locations`, `jobs` and the
rest) do not change. The new table is an index under "Rebuildable and user
data" in [ARCHITECTURE.md](../../ARCHITECTURE.md).

## Migration

Add `internal/store/migrations/00013_transcode_probes.sql`.

```sql
-- Probe data a live transcode needs. An index: if lost, the next transcode or a re-probe fills it again (requirement 10).
create table video_transcode_probes (
    video_id   integer primary key references videos (id) on delete cascade,
    -- domain.TranscodeProbeVersion. A different value reads as "absent" (see "Rules").
    version    integer not null,
    -- os.Stat size and modification time (Unix nanoseconds) of the probed file. Compared with the file opened at request time (see "When the value is written and read").
    size_bytes integer not null,
    mtime_ns   integer not null,
    -- JSON of domain.TranscodeProbe (see "Stored value: `domain.TranscodeProbe`").
    probe      text    not null,
    updated_at integer not null
) without rowid;
```

**Relationships**: the row is deleted by cascade when its `videos` row is
deleted. The same happens when a video's content changes and its row is
recreated; the new row is filled through the path of requirement 10.

## Stored value: `domain.TranscodeProbe`

A value type in `internal/domain`. It travels on `domain.Probe` as
`Transcode *TranscodeProbe`, and the ingest probe (`app.Ingest.Probe`) passes it
to the store as is. For a video with no usable video stream (not an attachment,
with dimensions) it is nil, and no row is written (that video cannot be
transcoded live).

| Field | Content | Used for |
| --- | --- | --- |
| `FormatName` | `format.format_name` (lowercase) | Deciding the two-input MOV case |
| `Video.Index` | `index` of the chosen video stream (the first non-attachment stream) | `-map` |
| `Video.CodecName`, `Profile`, `Level`, `PixelFormat`, `BitsPerRawSample` | Video coding | `videoCanCopy` |
| `Video.Width`, `Height`, `SampleAspectNum`, `SampleAspectDen`, `Rotation` | Geometry (rotation from the Display Matrix or the `rotate` tag) | Handling dimensions, aspect ratio and rotation |
| `Video.FPS`, `RealFPS` | `avg_frame_rate`, `r_frame_rate` | Detecting variable frame rate and capping fps |
| `Audio` (may be nil) | `Index`, `CodecName`, `Profile`, `SampleRate`, `Channels` of the chosen audio stream (the first stream) | `-map`, `audioCanCopy` |

The fields are the same as today's `transcodeMetadata`
([internal/media/transcode.go](../../internal/media/transcode.go)); the type
moves to `internal/domain`. JSON field names are the Go field names. Adding a
field or changing a field's meaning bumps `domain.TranscodeProbeVersion`.

## Rules

`domain.TranscodeProbeUsable` is a pure function. The stored value is used only
when all of the following hold:

1. The row exists.
2. `version` equals the current `domain.TranscodeProbeVersion`. (The edge case
   "the ffprobe output format changes" is handled by the version, because the
   stored shape is the parser's value.)
3. `probe` decodes as a `TranscodeProbe`.
4. `size_bytes` and `mtime_ns` equal the `Stat` of the file the transcode
   actually opened (requirement 9; for another location with the same content,
   the values of the opened location are compared).

Otherwise the value is treated as absent: ffprobe runs at request time and its
result replaces the row (requirements 9 and 10).

## When the value is written and read

| When | Operation | Identity |
| --- | --- | --- |
| Ingest probe job (`ApplyProbeForJob`) and `ApplyProbe` | Upsert in the same transaction as the `videos` update | The `os.Stat` that `media.Probe` takes right before ffprobe |
| Re-probe (`POST /api/videos/{id}/probe`) | Same as above (through a job) | Same as above |
| A live transcode runs ffprobe on the spot (`SaveTranscodeProbe`) | One upsert statement | `Stat` of the file the transcode opened |
| A live transcode starts (`LibraryStore.TranscodeProbe`) | Read one row | Compared as in [Rules](#rules) |

- The upsert is one statement (`insert … on conflict (video_id) do update`).
  When ingest and a transcode write at the same time, no corrupt value remains
  and the later write wins (edge case "several requests at once").
- An ffprobe that is cut off ends in an error and is never saved (edge case
  "the request is abandoned midway").
- Nothing backfills existing videos in bulk at startup or at ingest
  (requirement 10).
