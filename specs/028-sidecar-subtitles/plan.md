# Implementation Plan: Show subtitle files (SRT / WebVTT) placed next to a video on the playback screen

**Branch**: `feature/028-sidecar-subtitles` | **Parent Issue**: #522

**Input**: The parent Issue. It is this feature's specification.

## Summary

Subtitle files with the same name in the same folder as the video file
(`<name>.srt`, `<name>.vtt`, `<name>.<label>.srt`, `<name>.<label>.vtt`) can be
chosen and shown on the playback screen without settings or a rescan. In both
direct playback and live transcode, subtitles appear at the original video's
times.

| Concern | Approach |
| --- | --- |
| Finding | `GET /api/videos/{id}/subtitles`, called by the playback screen, reads the folder of the location that streaming opens on every call. Nothing is added to SQLite ([research.md R-1](research.md#r-1-find-subtitle-files-by-reading-the-folder-on-each-request-not-from-sqlite), [R-2](research.md#r-2-the-searched-folder-is-the-folder-of-the-location-that-streaming-opens)). Pure functions in `internal/domain` own the name matching, duplicate and limit (4 MiB) rules ([R-3](research.md#r-3-name-matching-and-duplicate-rules-live-in-pure-functions-in-internaldomain)). |
| Serving | `GET /api/videos/{id}/subtitles/{file}` uses a Go-only conversion in `internal/media` to detect the encoding (UTF-8, UTF-16 with BOM, Shift_JIS), convert SRT to WebVTT and shift the times by `offsetMs`. ffmpeg is not used ([R-4](research.md#r-4-conversion-in-go-without-ffmpeg), [R-5](research.md#r-5-character-encoding-is-decided-by-bom-then-utf-8-validity-then-shift_jis), [R-8](research.md#r-8-srt-format-variations-normalize-only-timing-lines-pass-cue-text-through)). The contract is [contracts/subtitles-api.md](contracts/subtitles-api.md). |
| Timing | In live transcode, each time the player settles the playback timeline's offset (the `transcode-start` report), it reattaches the tracks with that value as `offsetMs`. Nothing is attached until it is settled ([R-6](research.md#r-6-live-transcode-timing-the-server-returns-webvtt-shifted-by-offsetms)). |
| Screen | video.js's `SubsCapsButton` sits next to the playback speed, the selection (on/off and label) is remembered in the browser the same way as the volume, and the `c` key toggles it ([R-9](research.md#r-9-the-subtitle-selection-is-stored-in-websrcpreferences-like-the-volume), [R-10](research.md#r-10-subtitle-button-and-menu-use-videojs-subscapsbutton), [R-11](research.md#r-11-the-c-key-is-added-to-keyboardts-the-player-decides-the-toggle)). |

The visual and interaction standard is the parent Issue's `UI品質` section as
written (there is no `ui` label and no design stage). Strings are added to the
existing English catalog (requirement 12).

## Technical Context

**Canonical definitions**:

| Topic | Source |
| --- | --- |
| Boundaries and dependency direction, the rule for which files may be opened, the authentication boundary, caching of generated artifacts | [ARCHITECTURE.md](../../ARCHITECTURE.md), [internal/mediafs/media_file.go](../../internal/mediafs/media_file.go), [internal/httpapi/auth.go](../../internal/httpapi/auth.go) (`accessRoutes`), [internal/httpapi/artifact_cache.go](../../internal/httpapi/artifact_cache.go), [specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md) |
| Streaming and live transcode routes | [internal/httpapi/stream.go](../../internal/httpapi/stream.go) (`openMediaFile`), [internal/httpapi/transcode.go](../../internal/httpapi/transcode.go), [internal/httpapi/visibility.go](../../internal/httpapi/visibility.go) (`lookupServedVideo`) |
| The live transcode timeline and the report of the actual start position | [docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md), [specs/018-live-transcode-seek/contracts/transcode-start-api.md](../018-live-transcode-seek/contracts/transcode-start-api.md), [web/src/player/liveOffset.ts](../../web/src/player/liveOffset.ts) |
| The playback screen and the control bar | [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md) ("Control bar"), [web/src/player/VideoPlayer.tsx](../../web/src/player/VideoPlayer.tsx), [web/src/player/keyboard.ts](../../web/src/player/keyboard.ts), [web/src/player/playerControls.ts](../../web/src/player/playerControls.ts), [web/src/preferences/playbackVolume.ts](../../web/src/preferences/playbackVolume.ts), [docs/design-docs/library-ui.md](../../docs/design-docs/library-ui.md) |
| Screen strings and replacing video.js strings | [docs/design-docs/i18n.md](../../docs/design-docs/i18n.md), [web/src/i18n/en.ts](../../web/src/i18n/en.ts) (`player.controls`, `playerDictionary()`) |
| The API source of truth and the error shape | [api/openapi.yaml](../../api/openapi.yaml), [specs/023-english-i18n/contracts/error-api.md](../023-english-i18n/contracts/error-api.md) |
| Check entry points | [Taskfile.yml](../../Taskfile.yml) (`task check`, `task check-docs`, `task generate`, `task test-e2e`), [web/e2e/playback.e2e.ts](../../web/e2e/playback.e2e.ts), [web/e2e/media-fixtures.mjs](../../web/e2e/media-fixtures.mjs) |

**Feature-specific context**:

- No Go or npm dependency is added. Decoding uses `golang.org/x/text`
  (`encoding/unicode`, `encoding/japanese`), already a dependency (R-5).
  Subtitle display and the menu use `SubsCapsButton`, `addRemoteTextTrack` and
  `vjs-text-track-display`, which video.js 8 already has (R-10).
- No SQLite table, column, migration or domain event is added (R-1). There is
  no `data-model.md`.
- The subtitle file limit is `domain.SubtitleFileLimit = 4 MiB` (R-3; the value
  the parent Issue's edge case left to the plan).
- The live transcode timeline (the offset in `liveOffset.ts`, the
  `transcode-start` report) does not change. Subtitles only read that offset
  (R-6).
- No `ui-design.md` is created (there is no `ui` label). The screen standard is
  the parent Issue's `UI品質` section.

## Constitution Check

| Gate | Verdict |
| --- | --- |
| Dependency direction (ARCHITECTURE.md "Intended dependency direction") | Pass. See the package table below. |
| The rule for which files may be opened (the `internal/mediafs` paragraph in ARCHITECTURE.md) | Pass. `httpapi` runs neither `ReadDir` nor `Open` itself; it passes the location and the media folders to `mediafs`. Only names in the list are opened, and no path is built from the request string ([contracts, `GET /api/videos/{id}/subtitles/{file}`](contracts/subtitles-api.md#get-apivideosidsubtitlesfile)). |
| The authentication boundary (the authentication paragraph in ARCHITECTURE.md, requirement 11) | Pass. The two routes are added to `accessRoutes` as "guests too", and `security` in `openapi.yaml` and `openapi_routes_test.go` confirm they match. For guests, `lookupServedVideo` returns only public videos. |
| The API source of truth (ARCHITECTURE.md) | Pass. `api/openapi.yaml` changes, `task generate` runs, and generated files are not hand-edited (AGENTS.md). |
| Index versus user data (ARCHITECTURE.md "Rebuildable and user data") | Not applicable: nothing is stored. |
| Screen strings come from the catalog (docs/design-docs/i18n.md, requirement 12) | Pass. The strings for the button, the menu, "Off" and "Default" go into `en.ts`, and video.js strings are replaced by `playerDictionary()`. The pseudo-locale check catches omissions. |
| Server output is English (gosmopolitan in `.golangci.yml`) | Pass. Logs and `message` are English. |
| Documents are fixed in the same PR as the change (core-beliefs.md, AGENTS.md) | Pass. Each unit updates ARCHITECTURE.md, design documents and how-to guides. |

How the dependency direction holds, per package:

| Package | Role in this feature |
| --- | --- |
| `internal/domain` | Pure functions and values for name matching, duplicates, ordering and the limit (`SubtitleSidecar`). Has neither `os` nor `x/text/encoding`. |
| `internal/mediafs` | Lists folder entries and decides whether a name from the list may be opened (the single home of the read rules; R-2). |
| `internal/media` | Encoding detection, conversion to WebVTT and time shifting (format handling, like `fmp4.go`; R-4). Has no store or logger and returns errors as values. |
| `internal/httpapi` | Only parses requests, calls `lookupServedVideo`, calls through the `MediaFiles` and `SubtitleConverter` interfaces, and converts to `gen` types. |
| `cmd/mdm` | Wiring only. Sibling packages do not import each other (the depguard rules do not change). |
| `internal/app` | Not touched. One request completes the work, with no state or decision carried over (same as streaming). |

The verdicts are the same after Phase 1. There is no violation for Complexity
Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/028-sidecar-subtitles/
├── plan.md                     # This file
│                               # No spec.md — the parent Issue is the specification
├── research.md                 # R-1–R-11: finding, searched folder, matching rules, conversion, encoding,
│                               #   timing, broken files, SRT variations, remembering, button, c key
├── quickstart.md               # Human checks for full screen, Safari and a real Shift_JIS file
└── contracts/
    └── subtitles-api.md        # GET /api/videos/{id}/subtitles, GET …/subtitles/{file}
```

No `data-model.md` is created: no table or column is added and no entity
changes (R-1).

### Source Code

**Affected boundaries**:

| Boundary | What changes |
| --- | --- |
| `internal/domain` | `SubtitleSidecar`, `SidecarEntry`, `SubtitleFileLimit`, `SubtitleSidecars` (R-3) |
| `internal/mediafs` | `ListSidecarFiles`, `OpenSidecarFile` (R-2) |
| `internal/media` | `SubtitleConverter` (encoding, SRT → WebVTT, WebVTT check, `offsetMs`; R-4–R-8) |
| `api/openapi.yaml`, `internal/httpapi`, `cmd/mdm/main.go` | Two routes, `accessRoutes`, the `MediaFiles` interface and a new `SubtitleConverter` interface, an added `reason`; wiring in `main.go` |
| `web/src/api/client.ts`, `web/src/player`, `web/src/preferences`, `web/src/i18n/en.ts`, `web/src/index.css` | In `web/src/player`: `VideoPlayer.tsx`, `VideoPage.tsx`, `keyboard.ts`, `playerControls.ts`, `liveOffset.ts` |
| `web/e2e` | Subtitle fixtures and playback checks |
| `ARCHITECTURE.md`, `docs/design-docs/`, `docs/how-to/running-vv.md` | Documentation |

**New paths**:

| Path | Purpose |
| --- | --- |
| `internal/domain/subtitle.go` | Matching rules |
| `internal/mediafs/sidecar.go` | Listing and opening |
| `internal/media/subtitle.go` | Conversion |
| `internal/httpapi/subtitles.go` | Routes |
| `web/src/player/subtitleTracks.ts` | Track reattachment, selection and the `c` key decision, split out of `VideoPlayer.tsx` |
| `web/src/preferences/subtitlePreference.ts` | The remembered selection |
| `docs/design-docs/sidecar-subtitles.md` | The current design of finding, conversion and timing; listed in `docs/design-docs/index.md` |

**Structure decision**: Follows the existing layout
([ARCHITECTURE.md](../../ARCHITECTURE.md)). The conversion sits in
`internal/media`, and `internal/httpapi` receives it through an interface
(R-4). It does not go through `internal/app` because, as with the streaming and
live transcode routes, one request completes the work and app has no state or
decision to hold. Creating a "video subtitles" use case in app was rejected: it
only inserts one more interface and adds no decision.

## Implementation Work

### Find subtitle files next to a video, detect their encoding and convert them to WebVTT

**Scope**: The three components below HTTP.

- `internal/domain/subtitle.go`: `SidecarEntry` (name and size),
  `SubtitleSidecar` (file name, label, format), `SubtitleFileLimit`,
  `SubtitleSidecars`
  ([R-3](research.md#r-3-name-matching-and-duplicate-rules-live-in-pure-functions-in-internaldomain)).
- `internal/mediafs/sidecar.go`: `ListSidecarFiles(roots, videoPath)` returns
  the names and sizes of the regular files (excluding symlinks) in the video's
  folder, only when the video's location opens under the same rule as
  `OpenMediaFile`. `OpenSidecarFile(roots, videoPath, name)` opens `name` in the
  same folder and accepts only a `name` equal to an entry name in the folder
  ([R-2](research.md#r-2-the-searched-folder-is-the-folder-of-the-location-that-streaming-opens)).
- `internal/media/subtitle.go`:
  `SubtitleConverter.Convert(src []byte, format domain.SubtitleFormat, offsetMs int64) ([]byte, error)`.
  Encoding
  ([R-5](research.md#r-5-character-encoding-is-decided-by-bom-then-utf-8-validity-then-shift_jis)),
  SRT → WebVTT
  ([R-8](research.md#r-8-srt-format-variations-normalize-only-timing-lines-pass-cue-text-through)),
  the WebVTT header check, and the `offsetMs` shift
  ([R-6](research.md#r-6-live-transcode-timing-the-server-returns-webvtt-shifted-by-offsetms)).
  Unreadable input returns an error wrapping `domain.ErrSubtitleUnreadable`
  ([R-7](research.md#r-7-broken-files-appear-in-the-list-and-return-404-on-fetch)).

**Dependencies**: None.

**Acceptance**: The following tests exist and `task check` passes.

- `internal/domain` tests (table): for `movie.mp4`, `movie.srt`, `Movie.SRT`,
  `movie.ja.srt` and `movie.en.forced.vtt` are candidates, and `movie2.srt`,
  `movie.txt` and `other.srt` are not. For `my.movie.2024.mp4`, the label of
  `my.movie.2024.ja.srt` is `ja`. For an `.srt` and a `.vtt` with the same
  label, only the `.vtt`. Order is unlabelled, then labels in natural order.
  Entries over 4 MiB are excluded.
- `internal/mediafs` tests (temporary directory): files next to the video are
  listed, and subfolders and symlinks are not. A video outside the media
  folders can be neither listed nor opened. `OpenSidecarFile` refuses names not
  in the list (`../x.srt`, a name from another folder).
- `internal/media` tests (table): the same Japanese SRT in UTF-8 (with and
  without BOM), UTF-16 LE and BE with BOM, and Shift_JIS becomes the same
  WebVTT. A Shift_JIS sequence that is also valid UTF-8 (an SRT containing
  `E0 A1 A1`) is read as Shift_JIS and becomes `爍｡`; a Korean UTF-8 SRT is read
  as UTF-8; a BOM-less SRT containing a sequence for which the Shift_JIS decoder
  emits `U+FFFD` (`F0 40`) returns `ErrSubtitleUnreadable`. Also covered: `,`
  and `.` as the decimal separator, cues without a number line, CRLF, times
  with fewer digits, cue text containing `<i>` and `{\an8}` (passed through), a
  cue whose timing line cannot be read (dropped), and an SRT with no cue, a VTT
  without `WEBVTT`, and an empty file (`ErrSubtitleUnreadable`). With
  `offsetMs = 8000`, a 0–5 s cue disappears, a 3–10 s cue becomes 0–2 s, and a
  10–12 s cue becomes 2–4 s. WebVTT `NOTE` and `STYLE` blocks remain after the
  shift.

### Add the subtitle list and fetch API, returning only public videos' subtitles to guests

**Scope**: The two routes in
[contracts/subtitles-api.md](contracts/subtitles-api.md).

- `api/openapi.yaml`: `listVideoSubtitles`, `getVideoSubtitle`,
  `SubtitleTrack`, and `subtitle_unavailable` in `reason`. Run `task generate`.
- `internal/httpapi/subtitles.go`: looks up the video with
  `lookupServedVideo`, tries the locations in the same order as
  `openMediaFile`, calls `MediaFiles.ListSidecarFiles`, and builds the list with
  `domain.SubtitleSidecars`. The fetch opens a file only on an exact match with
  the list, and returns the result of `SubtitleConverter.Convert` as
  `text/vtt` with `private, no-cache` and a digest `ETag` (`hashETag`), and 304
  for `If-None-Match`. Failures are 404 `subtitle_unavailable` with a `Warn`
  log. The routes are added to `accessRoutes` as "guests too".
- `cmd/mdm/main.go`: wires `media.SubtitleConverter`.
- `web/src/api/client.ts`: `getVideoSubtitles(id)`,
  `subtitleUrl(id, file, offsetMs)`.
- Documents: ARCHITECTURE.md (remove "Not built yet: subtitles", add the routes
  to the descriptions of `internal/mediafs` and `internal/media`, and remove
  "subtitle conversion" from ffmpeg's role at the top), and
  `docs/design-docs/sidecar-subtitles.md` (finding, conversion, access control)
  with `index.md`.

**Dependencies**: `Find subtitle files next to a video, detect their encoding and convert them to WebVTT`.

**Acceptance**: The following tests exist, `task check` and `task check-docs`
pass, and `task generate` produces no diff.

- `internal/httpapi` tests (temporary directory and the real `mediafs`): with
  `movie.srt` and `movie.ja.srt` in place, the list returns 2 entries in the
  contract's order and shape; with none, an empty array. Adding `movie.en.vtt`
  after opening and calling again returns 3 (acceptance criterion 4). The fetch
  returns WebVTT as `text/vtt`, shifted by `offsetMs`. A name not in the list,
  an `.srt` hidden by a `.vtt`, and a broken file get 404
  `subtitle_unavailable`. `offsetMs=-1` gets 400. A matching `ETag` gets 304.
- `internal/httpapi/guest_test.go`, `openapi_routes_test.go`: a guest can fetch
  the list and the subtitles of a public video, and a non-public video gets 404
  `video_not_found` on both routes (acceptance criterion 11). The boundary
  handling matches `security` in `openapi.yaml`.

### Add a subtitle button and menu to the playback screen, remember the choice in the browser, and toggle it with the c key

**Scope**: Subtitles for videos in direct playback. The standard is the parent
Issue's `UI品質` section and requirements 5–8, 10 and 12.

- `web/src/player/VideoPage.tsx`: calls `getVideoSubtitles` each time a video
  opens and passes the result to `VideoPlayer`. A failure is treated as no
  subtitles. Moving to the previous or next video rebuilds from the new video's
  list.
- `web/src/player/VideoPlayer.tsx`, `subtitleTracks.ts`: put `subsCapsButton`
  before the playback speed, with `textTrackSettings: false`. Attach tracks
  with `addRemoteTextTrack` (`offsetMs = 0`), and when a label matches the
  stored value, set it to `showing`
  ([R-9](research.md#r-9-the-subtitle-selection-is-stored-in-websrcpreferences-like-the-volume),
  [R-10](research.md#r-10-subtitle-button-and-menu-use-videojs-subscapsbutton)).
  Store only when the user changes the selection in the menu. Add the video.js
  subtitle strings to `playerDictionary()`, and give the button
  `aria-keyshortcuts="C"` and `withKey(…, "C")`.
- `web/src/player/keyboard.ts`, `playerControls.ts`: `c` →
  `toggleSubtitles()`
  ([R-11](research.md#r-11-the-c-key-is-added-to-keyboardts-the-player-decides-the-toggle)).
  `menuOpen()` is also true while the subtitle menu is open (the
  `rateMenuOpen` check covers `.vjs-menu` in general; confirm it includes the
  subtitle menu).
- `web/src/preferences/subtitlePreference.ts`: total read and write functions.
- `web/src/index.css`: while the control bar is visible, align the bottom edge
  of `vjs-text-track-display` with the height of the control bar and the
  progress bar (requirement 10).
- `web/src/i18n/en.ts`: `player.subtitles` (the button name, off, the default
  subtitle name).
- Documents: add the subtitle layer to the playback screen structure in
  `docs/design-docs/library-ui.md`, and describe how to place subtitle files
  (the name shapes, the supported formats and encodings) in
  `docs/how-to/running-vv.md`.

**Dependencies**: `Add the subtitle list and fetch API, returning only public videos' subtitles to guests`.

**Acceptance**: This unit changes a screen, so the look and interaction are
checked at 360px, 768px and 1280px widths and in full screen
([quickstart.md, Full screen and overlap with the control bar (acceptance criterion 10)](quickstart.md#full-screen-and-overlap-with-the-control-bar-acceptance-criterion-10)).
The following tests exist, and `task check` and `task test-e2e` pass.

- `web/src/player` unit tests: a video with no subtitles has no subtitle button
  (acceptance criterion 3). With 2 subtitles, the button and the menu show
  `ja`, `en` and "Off", and there is no subtitle settings item (acceptance
  criterion 2). An unlabelled subtitle appears with the catalog's default name
  (acceptance criterion 12). Without a stored value, subtitles start off
  (acceptance criterion 7). With the stored value
  `{ enabled: true, label: "ja" }`, a video with `ja` has `ja` `showing`, a
  video without it is off, and the stored value does not change (acceptance
  criterion 6). Choosing in the menu changes the stored value. `c` turns
  subtitles off when one is showing; when off, it shows the stored label, or
  else the first track; on a video without subtitles nothing happens
  (acceptance criterion 8). The pseudo-locale check (`expectCatalogTextOnly`)
  passes.
- `web/src/preferences` unit tests: a broken value or an unreadable storage
  falls back to the default, and a failed write does not throw.
- `web/e2e/playback.e2e.ts`: with `direct.srt` and `direct.ja.srt` next to the
  fixture `direct.mp4`, the button appears; choosing a subtitle shows its text
  in `vjs-text-track-display` at the cue time, and "Off" removes it (acceptance
  criteria 1 and 2). After choosing `ja` and reloading, `ja` stays on
  (acceptance criterion 6).

### Align subtitles to the original video's time during live transcode playback

**Scope**: Reattaching tracks to match the playback timeline's offset
([R-6](research.md#r-6-live-transcode-timing-the-server-returns-webvtt-shifted-by-offsetms),
[contracts, Client use](contracts/subtitles-api.md#client-use)).

- `web/src/player/liveOffset.ts`: add `vvOffsetSettled(seconds)` to
  `LiveSource`, which reports that the offset is settled (a 200 report, a 404
  or an error, or a source without `attempt`). Seeks that rebuild the source
  (`reloadAt`) report through the same path.
- `web/src/player/VideoPlayer.tsx`, `subtitleTracks.ts`: when the offset
  becomes unsettled, remove the tracks; when it settles, reattach them with
  `offsetMs` on the URL and set the label shown just before to `showing`. The
  stored value is not rewritten. Switching from direct playback to live
  transcode (`fallbackToTranscode`) works the same.
- Documents: add a timing section to `docs/design-docs/sidecar-subtitles.md`,
  and point from "the report path" in `docs/design-docs/live-transcode-seek.md`
  to subtitles reading the offset.

**Dependencies**: `Add a subtitle button and menu to the playback screen, remember the choice in the browser, and toggle it with the c key`.

**Acceptance**: The following tests exist, and `task check`,
`task check-docs` and `task test-e2e` pass.

- `web/src/player/liveOffset.test.ts`: for a 200 report, a 404 and no
  `attempt`, `vvOffsetSettled` is called once each, with the actual start
  position for 200 and the requested position otherwise. It is not called for
  a stale report that arrives after the source was replaced.
- `web/src/player` unit tests: while the report is pending there is no track;
  when it arrives, tracks are attached with that offset's URL, the label that
  was showing stays `showing`, and the stored value does not change.
- `web/e2e/playback.e2e.ts`: next to an H.264 MKV with keyframes at 0, 8 and
  16 s (an existing fixture), place an SRT with cues at 6–7 s, 9–10 s and
  14–15 s. With subtitles on, seeking to about 14 s fetches the subtitle URL
  with `offsetMs=8000` after the report (8 s) arrives, the 9–10 s cue appears
  at 9–10 s on the display, and the 6–7 s cue does not appear. The same holds
  when a reload resumes from the saved position (acceptance criterion 9).
