# Contract: Sidecar subtitle files

Source of truth: `api/openapi.yaml`, operations `listVideoSubtitles` and
`getVideoSubtitle`. This document covers only the two routes this feature adds;
existing response shapes do not change.

Both routes have the same `security` as `getVideoStream` (owner and guest) and
are listed as "guests too" in `accessRoutes` in `internal/httpapi/auth.go`. The
video is looked up with `lookupServedVideo`, so a guest pointing at a video that
is not public gets the same 404 `video_not_found` as for a missing video
(requirement 11 of the parent Issue).

## 1. `GET /api/videos/{id}/subtitles`

`operationId: listVideoSubtitles`. The playback screen calls it each time it
opens a video.

**Response**: 200 with `{ "subtitles": SubtitleTrack[] }`, an empty array when
there are none. `SubtitleTrack` has `required: [file, label, format]` and
`additionalProperties: false`:

| Field | Type | Meaning |
| --- | --- | --- |
| `file` | string | The subtitle file name (without the folder). Used as `{file}` in §2. |
| `label` | string | The label from the file name (`ja`, `en.forced`); `""` for an unlabelled subtitle. |
| `format` | `srt` \| `vtt` | The format of the original file. |

- Order and duplicate rules are in
  [research.md R-3](../research.md#r-3-name-matching-and-duplicate-rules-live-in-pure-functions-in-internaldomain)
  (unlabelled first, then labels in natural order; for the same label in `.srt`
  and `.vtt`, only the `.vtt`; files over 4 MiB are not listed).
- The searched folder is the folder of the location that streaming opens
  ([R-2](../research.md#r-2-the-searched-folder-is-the-folder-of-the-location-that-streaming-opens)).
- File content is not read. Broken files are listed here too and get 404 in §2
  ([R-7](../research.md#r-7-broken-files-appear-in-the-list-and-return-404-on-fetch)).
- The response has `Cache-Control: no-store` and returns the folder's current
  state every time the video is opened (requirement 2).

| Status | `code` / `reason` | When |
| --- | --- | --- |
| 200, empty array | — | The folder cannot be read. The cause is logged; this is not distinguished from having no subtitles, and playback itself does not stop. |
| 404 | `reason` `file_unavailable` | No location opens (same as `getVideoStream`). |
| 404 | `video_not_found` | The video does not exist, or a guest pointed at a video that is not public. |

## 2. `GET /api/videos/{id}/subtitles/{file}`

`operationId: getVideoSubtitle`. Takes a `file` returned by §1.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `file` | path | string | Yes | A name §1 returned. The server rebuilds the same list as §1 and opens `file` only when it equals one entry (exact string match). A name not in the list, an `.srt` hidden by a `.vtt`, and a name over the limit all get 404. No path-separator or `..` check is needed, because nothing outside the list is opened. |
| `offsetMs` | query | integer, `minimum: 0` | No (default 0) | Which time in the original video, in milliseconds, the playback timeline's 0 is. Subtracted from the time of every cue. A cue whose end time becomes 0 or less is not returned; a cue whose start time becomes negative starts at 0 ([R-6](../research.md#r-6-live-transcode-timing-the-server-returns-webvtt-shifted-by-offsetms)). A negative or non-integer value is 400 `invalid_request`. |

**Response**: 200 with `Content-Type: text/vtt; charset=utf-8` and a UTF-8
WebVTT body. SRT is converted
([R-8](../research.md#r-8-srt-format-variations-normalize-only-timing-lines-pass-cue-text-through));
WebVTT has its header checked and is shifted by `offsetMs`. The encoding is
decided in the order of
[R-5](../research.md#r-5-character-encoding-is-decided-by-bom-then-utf-8-validity-then-shift_jis).
The response has `Cache-Control: private, no-cache` and an `ETag` that is a
digest of the converted body; a matching `If-None-Match` gets 304 (the same
handling as generated artifacts, guest-api.md §5).

| Status | `code` / `reason` | When |
| --- | --- | --- |
| 404 | `reason` `subtitle_unavailable`, `code` `not_found` | `file` is not in the list, cannot be opened, is empty, exceeds the limit, cannot be decoded, has no readable SRT cue, or lacks the WebVTT header. The cause is logged at `Warn` on the server ([R-7](../research.md#r-7-broken-files-appear-in-the-list-and-return-404-on-fetch)). |
| 404 | `video_not_found` / `file_unavailable` | Same as §1. |
| 400 | `invalid_request` | `offsetMs` is malformed. |

## 3. Client use

- The playback screen calls §1 when it opens a video (around creating
  `VideoPlayer`) and passes the result to `VideoPlayer`. When the call fails,
  the video is treated as having no subtitles and playback does not stop.
- Each time the playback timeline's offset is settled, `VideoPlayer` removes the
  existing subtitle tracks and reattaches them with that offset as `offsetMs` on
  the §2 URL. The offset is 0 for direct playback. For live transcode, nothing
  is attached while `liveOffset.ts` waits for the `transcode-start` report;
  tracks are attached with the offset at the moment the report arrives (or is
  settled to the requested position by a 404). Seeks that rebuild the source
  behave the same.
- The subtitle (label) that was showing before reattachment is set to
  `showing` on the reattached track. Reattachment does not rewrite the stored
  value
  ([R-9](../research.md#r-9-the-subtitle-selection-is-stored-in-websrcpreferences-like-the-volume)).
