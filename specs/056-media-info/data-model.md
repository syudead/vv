# Data model: Media information from the import probe

The rest of the model is unchanged: the `videos` columns the probe fills, the
live-transcode row and the job queue are
[018 data-model](../018-live-transcode-seek/data-model.md),
[020 data-model](../020-seek-thumbnail-stage/data-model.md) and the migrations
in [`internal/store/migrations`](../../internal/store/migrations).

## Migration

Two migrations, each numbered after the last one on `main` when its unit is
written (other feature branches add migrations too).

| Migration | Unit | What it does |
| --- | --- | --- |
| `video_media_info` | Read and store media information at import | Creates [`video_media_info`](#video_media_info) |
| `metadata_jobs` | Re-analyse imported videos in the background | Rebuilds `jobs` with `metadata` in the `kind` check, as `00025_video_fingerprints.sql` did, and queues one `metadata` job per video with `probe_state = done` and a location under a registered folder, skipping a video that already has a queued or running one |

Both tables are a rebuildable index in
[Data and recovery](../../docs/how-to/running-vv.md#data-and-recovery): a scan
and the jobs it queues fill them again.

## `video_media_info`

One row per video, written by the probe job and the `metadata` job
([R-1](research.md#r-1-media-information-is-one-versioned-json-row-per-video)).

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| `video_id` | integer | No | Primary key; `references videos (id) on delete cascade` |
| `version` | integer | No | `domain.MediaInfoVersion` when written; a different value reads as absent |
| `info` | text | No | JSON of [`domain.MediaInfo`](#stored-value-domainmediainfo) |
| `updated_at` | integer | No | Unix seconds of the write |

**Relationships**: the row goes with its `videos` row. A content change at a
path recreates the video row (`UpsertVideo`), so the new row starts without
media information and its probe job writes it.

## Stored value: `domain.MediaInfo`

A value type in `internal/domain`, filled by `internal/media` from the one
`ffprobe` run ([R-2](research.md#r-2-the-same-ffprobe-run-reads-chapters-and-reads-longer-for-mpeg-ts))
and, for a TS file, the SI reader
([R-4](research.md#r-4-station-and-programme-names-of-a-ts-recording-come-from-a-go-si-reader-with-an-arib-std-b24-decoder)).
It travels on `domain.Probe` as `Media *MediaInfo`. JSON field names are the
Go field names; an absent item is omitted from the JSON (`omitempty`, a nil
pointer or an empty slice). `MediaInfoVersion` starts at 1 and is bumped when
a field is added or its meaning changes.

The video stream is the first non-attachment video stream, the one the index
columns already describe.

| Field | Source in `ffprobe` | Absent when |
| --- | --- | --- |
| `Video.FrameRate` (float64) | `avg_frame_rate` of the video stream | `0/0` or unparsable |
| `Video.BitDepth` (int) | `bits_per_raw_sample`, else the depth in `pix_fmt` ([R-3](research.md#r-3-hdr-projection-3d-and-interlacing-are-decided-by-pure-rules-in-internaldomain)) | Neither gives a number |
| `Video.BitRate` (int64, bits per second) | The video stream's `bit_rate`, else its `tags.BPS` (Matroska) | Neither present |
| `Video.Interlaced` (*bool) | `field_order` | `unknown` or absent |
| `Video.HDR` (`sdr`, `hdr10`, `hlg`, `dolby_vision`) | `color_transfer`, `side_data_list`, `codec_tag_string` (R-3) | Never: a video stream is at least `sdr` |
| `Video.Projection` (string) | `side_data_list` entry `Spherical Mapping`, its `projection` | No such entry |
| `Video.Stereo3D` (string) | `side_data_list` entry `Stereo 3D`, its `type` | No such entry |
| `Video.ClosedCaptions` (bool) | `closed_captions` of the video stream | `0` |
| `AudioTracks[]` | Every `audio` stream, in stream order | No audio stream |
| `AudioTracks[].Index` | `index` | Never |
| `AudioTracks[].Codec` | `codec_name` | Empty |
| `AudioTracks[].Channels` (int), `ChannelLayout` (string) | `channels`, `channel_layout` | `0`, empty |
| `AudioTracks[].Language`, `Title` | `tags.language`, `tags.title` | Empty or `und` |
| `AudioTracks[].Default` (bool) | `disposition.default` | `0` |
| `SubtitleTracks[]` | Every `subtitle` stream, in stream order | No subtitle stream |
| `SubtitleTracks[].Index`, `Codec`, `Language`, `Title`, `Default` | As for audio | As for audio |
| `SubtitleTracks[].Forced` (bool) | `disposition.forced` | `0` |
| `SubtitleTracks[].HearingImpaired` (bool) | `disposition.hearing_impaired` | `0` |
| `Embedded.Title`, `Comment`, `Description`, `Genre`, `Date` | `format.tags` `title`, `comment`, `description` (else `synopsis`), `genre`, `date` | Empty; `Date` is kept as the file's text |
| `Embedded.Author` | `format.tags` `artist`, else `author`, else `album_artist` | All empty |
| `Embedded.RecordedAt` (RFC 3339 string) | `format.tags` `com.apple.quicktime.creationdate`, else `creation_time` | Neither parses as a date ([Rules](#rules)) |
| `Embedded.Device` | `com.apple.quicktime.make` and `com.apple.quicktime.model` joined by a space, else `com.android.manufacturer` and `com.android.model`, else `make` and `model` | All empty |
| `Embedded.Location` (`{Latitude, Longitude, Altitude *float64}`) | `com.apple.quicktime.location.ISO6709`, else `location`, else `location-eng`, parsed as ISO 6709 | None parses |
| `Embedded.CoverArt` (bool) | A stream with `disposition.attached_pic = 1` | None |
| `Broadcast.Station`, `Broadcast.Program` | The SI reader's SDT service name and EIT present event name (R-4) | Not a TS file, or not found in the first 32 MiB |
| `Chapters[]` | `chapters`, after the [Rules](#rules) | No chapter |
| `Chapters[].StartMs`, `EndMs` (int64) | `start_time`, `end_time` in milliseconds | Never |
| `Chapters[].Title` | `tags.title` | Empty |

## Rules

| Rule | Enforced in |
| --- | --- |
| A row whose `version` differs from `MediaInfoVersion` reads as absent, and the scan queues a `metadata` job for the video (`IndexedVideo.MediaInfoMissing`: `probe_state = done` and no row at the current version) | `store` (the read and the scan's index query), `scanner` (`ensurePendingJobs`) |
| The probe job writes the row in the `ApplyProbeForJob` transaction, under the same identity check as the `videos` update (job location id, version and path, content key, location generation) | `store` |
| The `metadata` job writes only the row, under the same identity check; when the check fails nothing is written and the job completes | `store` (`ApplyMediaInfoForJob`), `app` |
| A `metadata` job is claimable only when no `probe` job is claimable (`JobClaimCondition.NoClaimableProbe`) | `domain` (the condition), `store` (the claim query) |
| A `metadata` job that fails at the attempt limit leaves the row as it was; the issue kind is `metadata_failed` | `store` (`FailClaimedJob`) |
| Every string is trimmed, must be valid UTF-8 with control characters other than newline removed, and is cut at 2,000 characters; an empty result is absent | `domain` |
| At most 64 audio tracks, 64 subtitle tracks and 1,000 chapters are kept, in stream or time order | `domain` |
| `RecordedAt` parses RFC 3339 with or without an offset and `YYYY-MM-DD HH:MM:SS`; a value with an offset keeps it, one without is UTC; anything else is absent | `domain` |
| `Location` parses ISO 6709 `±DD.DDDD±DDD.DDDD[±ALT]/` with 2, 4 or 6 integer digits in the coordinates; latitude outside ±90 or longitude outside ±180 is absent | `domain` |
| Chapters are sorted by start; a chapter with `StartMs >= DurationMs` is dropped; `EndMs` is cut to `DurationMs` and to at least `StartMs`; a single chapter is kept; overlapping chapters are kept as given | `domain` (`NormalizeChapters`) |
| A missing or unreadable item is omitted and never fails the probe; only a missing duration fails it, as today | `media` (the parser), `domain` |

## What does not change

| Item | Why |
| --- | --- |
| `videos.duration_ms`, `width`, `height`, `video_codec`, `audio_codec`, `playable` | The index columns, playability and the seek preview keep their inputs; the `metadata` job never touches them |
| `video_transcode_probes` | The live transcode reads its own row; `MediaInfo` is not an input to any `ffmpeg` argument |
| `video_locations.file_created_at`, the title and the display name | Requirement 10: the embedded title and `RecordedAt` are shown beside them, not written into them |
| `Video.FileTitle` sorting and search keys | No search key is built from media information (out of scope) |
