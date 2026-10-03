# Sidecar subtitle files

- Status: adopted
- Scope: how SRT and WebVTT subtitle files in the video's folder are found,
  conversion to WebVTT, access control for `GET /api/videos/{id}/subtitles` and
  `GET /api/videos/{id}/subtitles/{file}`, and subtitle timing during live
  transcoding playback
- Background: [specs/028-sidecar-subtitles/research.md](../../specs/028-sidecar-subtitles/research.md);
  contract: [contracts/subtitles-api.md](../../specs/028-sidecar-subtitles/contracts/subtitles-api.md)

## Discovery

Subtitles are not indexed; they are found by reading the folder on each
request. No SQLite table or import job is involved. Adding or removing a
subtitle in the folder shows up in the next listing without a rescan.

1. `internal/httpapi/subtitles.go` tries the video's locations in the same order
   as delivery (`openMediaFile`). `ListSidecarFiles` in `internal/mediafs`
   returns the names and sizes of the regular files in the folder of a location
   (the path before following symlinks) only when that location opens under the
   same rules as `OpenMediaFile`. Subfolders and symlinks are excluded. Only the
   folder of the first location that opens is used, so when a video is in
   several places, only the files next to the one used for playback count.
2. `domain.SubtitleSidecars` is a pure function that builds the subtitle list
   from the video's file name and the entries.
   - `<name>` is the video file name without its last extension. Candidates are
     `<name>.srt`, `<name>.vtt`, `<name>.<label>.srt` and `<name>.<label>.vtt`;
     `<name>` and the extension are compared after NFC normalization, case
     insensitively. A label may contain dots (`en.forced`).
   - Entries larger than `domain.SubtitleFileLimit` (4 MiB) are not candidates.
   - When a `.srt` and a `.vtt` have the same label, only the `.vtt` remains.
   - The entry without a label comes first, then labels in natural order.
3. Listing does not read the contents. A broken file is listed and returns 404
   when fetched.

| Situation | List response |
| --- | --- |
| No location opens | 404 `file_unavailable` (same as delivery) |
| A location opens but its folder cannot be read | An empty list, as if there were no subtitles; the reason is logged |

List responses carry `Cache-Control: no-store`.

## Fetching and conversion

`GET /api/videos/{id}/subtitles/{file}` rebuilds the list and opens the file
with `OpenSidecarFile` only when `file` exactly equals one of the listed names
as a string. `OpenSidecarFile` also accepts only listed names and reapplies the
`OpenMediaFile` rules before opening, so even if a file is replaced by a symlink
after listing, nothing outside the registered folder is opened. The path is
never built from the request string, so no `..` or separator check is needed.
At most the limit (4 MiB) is read, even if a file grew after listing.

`SubtitleConverter` in `internal/media` converts in Go only; it does not start
`ffmpeg`. `internal/httpapi` receives it through a `SubtitleConverter` interface
it declares, and `cmd/mdm` wires it. One request reads, converts and returns,
so `internal/app` is not involved.

- Character encoding: a BOM (UTF-8, UTF-16 LE/BE) decides it when present.
  Otherwise, in order: valid UTF-8 consisting only of characters from scripts
  used in subtitles; Shift_JIS that produces neither `U+FFFD` nor C1 control
  characters; valid UTF-8. If none fits, the file is unreadable.
- SRT: the sequence-number lines are dropped, time lines are rewritten into
  WebVTT form, and cue text passes through unchanged. Cues with an unreadable
  time line are dropped. WebVTT passes through after its header is checked.
- `offsetMs` (default 0) is subtracted from every cue time. Cues whose end is 0
  or less are not returned; cues whose start becomes negative start at 0. This
  aligns subtitles to the time axis of the live transcoding output; the player
  passes the actual start position.

The response has `Content-Type: text/vtt; charset=utf-8`,
`Cache-Control: private, no-cache` and an `ETag` that is a digest of the
converted body; a matching `If-None-Match` returns 304.

| Case | Response |
| --- | --- |
| Name not in the list, a `.srt` hidden by a `.vtt`, a file over the limit, a file that cannot be opened or read | 404 `subtitle_unavailable`; the reason is logged at `Warn` |
| `offsetMs` negative or not an integer | 400 `invalid_request` |

## Access control

Both routes are "guests too" routes (the `security` of `api/openapi.yaml` allows
the owner and guests; `accessRoutes` in `internal/httpapi/auth.go` mirrors it)
and look the video up with `lookupServedVideo`. A guest asking for a video that
is not public gets the same 404 `video_not_found` as for a video that does not
exist. Guest requests are recorded in the same ledger as delivery, so making a
video private also cuts off requests in progress.

## Live transcoding time alignment

Video.js selects subtitle cues by the `<video>` element's `currentTime`, the
time axis of the transcoding output, both in emulation and with the browser's
native tracks. The offset that the `liveOffset.ts` mediator adds to the current
time does not apply to subtitles, so the server returns WebVTT shifted by
`offsetMs`, and the player reattaches the track each time the offset is settled
([research.md R-6](../../specs/028-sidecar-subtitles/research.md)).

- A `liveSource` source calls `vvOffsetSettled(seconds)` once each time it is
  settled which time of the original video is 0 on the playback time axis. A
  source without `attempt` (from the start) calls it immediately with the
  requested position; a source with `attempt` calls it with the actual start
  position when the `transcode-start` report returns 200, and with the
  requested position on 404 or an error. It calls `vvOffsetPending()` when it
  starts waiting for the report. Rebuilding for an unbuffered seek
  (`reloadAt`) goes through the same path, and a stale report arriving after
  the source was replaced triggers no call
  ([start position report in live-transcode-seek.md](live-transcode-seek.md#start-position-report)).
- `VideoPlayer.tsx` passes both to `setOffset` in `subtitleTracks.ts`. While
  unsettled (`null`), the track is removed so that shifted subtitles never show,
  even briefly. Once settled, the track is reattached with
  `subtitleUrl(id, file, offsetMs)`, and the label shown just before is set to
  `showing`. Reattaching is not a user choice, so it does not overwrite the
  saved value. Only the first attachment after the list changes uses the saved
  value to decide what to show.
- Direct playback keeps an offset of 0. Switching from direct playback to
  transcoding (`fallbackToTranscode`) also goes through `liveSource`, so it
  aligns by the same path.
- Reattaching happens only when transcoding restarts (which takes a few seconds
  anyway). Meanwhile the subtitle button is hidden too, because there is no
  track.
