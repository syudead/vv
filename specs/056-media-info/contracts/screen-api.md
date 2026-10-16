# Contract: Screen API for media information

Source of truth: [`api/openapi.yaml`](../../../api/openapi.yaml). This document
holds only what this feature adds or changes. The error shape is
[023 error-api.md](../../023-english-i18n/contracts/error-api.md).
`api/external-v1.yaml` is unchanged: it returns no technical facts about a
video today, and sorting or filtering by the new items is out of scope.

## `GET /api/videos/{id}`: `mediaInfo`

`Video` gains `mediaInfo` (`MediaInfo`), present only in this operation and
only when the video has a `video_media_info` row at the current version
([R-6](../research.md#r-6-mediainfo-is-part-of-the-video-detail-only-for-owners-and-guests-alike)).
A guest reading a public video gets the same value as the owner. List items,
related videos, library items and the responses of the owner's `PUT`s omit it.

Every property of `MediaInfo` and of its objects is optional and omitted when
the file has no such item ([data-model.md](../data-model.md)); an object with
no property is omitted whole. The page therefore shows a group only when it
has something to show.

| Property | Type | Meaning |
| --- | --- | --- |
| `video` | `MediaInfoVideo` | The video stream's items |
| `audioTracks` | `MediaInfoAudioTrack[]` | Every audio stream, in stream order |
| `subtitleTracks` | `MediaInfoSubtitleTrack[]` | Every subtitle stream, in stream order |
| `embedded` | `MediaInfoEmbedded` | Tags embedded in the file |
| `broadcast` | `MediaInfoBroadcast` | A TS recording's station and programme |
| `chapters` | `MediaInfoChapter[]` | Chapters, sorted by start, within the duration |

`MediaInfoVideo`:

| Property | Type | Meaning |
| --- | --- | --- |
| `frameRate` | number | Frames per second (`avg_frame_rate`) |
| `bitDepth` | integer | Bits per sample |
| `bitRate` | integer (int64) | Bits per second of the video stream |
| `interlaced` | boolean | Present only when the file says progressive or interlaced |
| `hdr` | `sdr` \| `hdr10` \| `hlg` \| `dolby_vision` | Always present when `video` is |
| `projection` | string | `equirectangular`, `cubemap` or another `ffprobe` projection name of a 360-degree video |
| `stereo3d` | string | `side by side`, `top and bottom` or another `ffprobe` stereo type |
| `closedCaptions` | boolean | `true` when the video stream carries closed captions |

`MediaInfoAudioTrack` and `MediaInfoSubtitleTrack`:

| Property | Type | Audio | Subtitle | Meaning |
| --- | --- | --- | --- | --- |
| `index` | integer | Required | Required | The stream index in the file |
| `codec` | string | Yes | Yes | `ffprobe` `codec_name` (`aac`, `subrip`, `arib_caption`) |
| `channels` | integer | Yes | No | Channel count |
| `channelLayout` | string | Yes | No | `stereo`, `5.1` and the like |
| `language` | string | Yes | Yes | The file's language tag, `und` omitted ([R-8](../research.md#r-8-language-codes-are-shown-through-intldisplaynames)) |
| `title` | string | Yes | Yes | The track's name |
| `default` | boolean | Yes | Yes | The default disposition |
| `forced` | boolean | No | Yes | The forced disposition |
| `hearingImpaired` | boolean | No | Yes | The hearing-impaired disposition |

`MediaInfoEmbedded`:

| Property | Type | Meaning |
| --- | --- | --- |
| `title`, `author`, `comment`, `description`, `genre` | string | The file's tags |
| `date` | string | The file's `date` tag as text |
| `recordedAt` | string (date-time) | The shooting time, with the file's offset when it has one |
| `device` | string | Make and model |
| `location` | `{latitude: number, longitude: number, altitude?: number}` | The shooting location |
| `coverArt` | boolean | `true` when the file carries an attached picture |

`MediaInfoBroadcast`: `station` and `program`, strings.

`MediaInfoChapter`: `startMs` and `endMs` (integer, int64, required), `title`
(string, optional).

**Response**: `200` with `Video` as today; `404` as today.

## Enumerations

No enumeration gains a value: the `metadata` job kind and `metadata_failed`
were removed with the re-analysis ([R-5](../research.md#r-5-media-information-is-read-only-by-the-import-probe-of-new-or-changed-files)).

## `GET /api/events`: `video`

No new kind. A finished probe job sends the `video` notification for its
video as every ingest stage does, so an open video page refetches the detail
and shows the new `mediaInfo`.

## Client use

The video page reads `mediaInfo` from the detail it already fetches; the
chapters go to the player as a `chapters` text track
([R-7](../research.md#r-7-chapters-are-a-chapters-text-track-with-marks-drawn-in-the-videojs-seek-bar)).
A language is shown by name through `Intl.DisplayNames` (R-8).
