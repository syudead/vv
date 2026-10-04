# Research: Sidecar subtitle files shown next to the video

Inherited decisions: the tech stack, the boundaries and dependency direction,
the rule for which files may be opened, and the live transcode timeline follow
the canonical documents
([docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md),
[ARCHITECTURE.md](../../ARCHITECTURE.md),
[internal/mediafs](../../internal/mediafs/media_file.go),
[docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md)).
This file records only the decisions this feature adds.

## R-1: Find subtitle files by reading the folder on each request, not from SQLite

**Decision**: `GET /api/videos/{id}/subtitles`, which the playback screen calls
([contracts/subtitles-api.md, `GET /api/videos/{id}/subtitles`](contracts/subtitles-api.md#get-apivideosidsubtitles)),
builds the subtitle list by running `ReadDir` on the video's folder on every
call. No table or column is added, and neither the scan nor the import job is
involved.

**Rationale**: Requirement 2 says subtitles are found "without a rescan" and
"reflected when the video is opened again". The playback screen runs one
`ReadDir` of one folder each time it opens, which is negligible next to the cost
of opening the video itself. Keeping subtitles in the index would need a
mechanism that tracks the state of files that change independently of the scan
(a watcher, or a comparison on every open).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Record whether subtitles exist in `video_locations` during the scan | Rejected: violates requirement 2; changes are not reflected until a rescan. |
| Include subtitles in the `GET /api/videos/{id}` response | Rejected: `Video` is also returned for lists, related videos and refetches during import, so every one of them would read the folder. Only the playback screen uses subtitles. |

## R-2: The searched folder is the folder of the location that streaming opens

**Decision**: Subtitles are searched for in the folder of the first location
that opens, trying locations in the same order as `openMediaFile` in
`StreamVideo` and `TranscodeVideo`. The same rule goes into `internal/mediafs`
as `ListSidecarFiles` (the names and sizes of the regular files in the folder
whose names start with the video's `<name>`) and `OpenSidecarFile` (opens one of
the names the list returned). `internal/httpapi` only passes the location and
the media folders.

**Rationale**: The parent Issue's edge case "the video is in several places"
decides "only next to the place used for playback". `openMediaFile` decides
the place used for playback, so subtitles go through the same check in the same
order. Whether a folder may be read is a rule that `internal/mediafs` owns in
one place; `httpapi` must not run `ReadDir` itself (ARCHITECTURE.md).

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Use the folder of `Video.location` (the representative location in the detail response) | Rejected: the representative location is chosen without checking that it opens, and playback uses another location when it does not. The playback folder and the subtitle folder would differ. |
| Search the folders of all locations together | Rejected: contradicts the edge case. |

## R-3: Name matching and duplicate rules live in pure functions in `internal/domain`

**Decision**: `domain.SubtitleSidecars(videoFileName string, entries []SidecarEntry) []SubtitleSidecar`
builds the subtitle list (file name, label, format) from the folder entries.
The rules:

| Rule | Behaviour |
| --- | --- |
| `<name>` | The video's file name without its last extension only (`my.movie.2024.mp4` → `my.movie.2024`). |
| Candidates | `<name>.srt`, `<name>.vtt`, `<name>.<label>.srt`, `<name>.<label>.vtt`. The `<name>` part and the extension are matched after Unicode normalization (NFC), ignoring case. |
| `<label>` | The non-empty remainder; it may contain dots (`en.forced`). The label is displayed as written in the file name. |
| Size limit | An entry larger than `domain.SubtitleFileLimit` (4 MiB) is not a candidate. |
| Same label in `.srt` and `.vtt` (case-insensitive) | Only the `.vtt` is kept. |
| Same extension differing only in case (possible on Linux) | The one first in natural name order is kept. |
| Order | Unlabelled first, then labels in natural order (`domain.CompareNatural`). |

**Rationale**: As a pure function, the case, multiple-dot, duplicate and limit
cases can be tested in a table without building a file system.
`internal/mediafs` owns only "may this be read", not what names mean. The
4 MiB limit was chosen because the SRT of a two-hour film is a few hundred KB
and one TV episode is about 100 KB: the value keeps every real subtitle file
and stops a large misplaced file from being read into memory.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| A 1 MiB limit | Rejected: a dialogue-heavy feature film or an SRT with many styling tags can exceed it. |
| No limit | Rejected: one request reads a whole broken or misplaced large file. |
| Show both entries of a duplicate label | Rejected: violates requirement 6 of the parent Issue. |

## R-4: Conversion in Go, without ffmpeg

**Decision**: `SubtitleConverter.Convert(src []byte, format, offsetMs) ([]byte, error)`
in `internal/media` detects the character encoding and converts to UTF-8,
converts SRT to WebVTT, checks the header of WebVTT and passes it through, and
in both cases shifts the times by `offsetMs` (R-6). It starts no external
process. `internal/httpapi` receives it through a `SubtitleConverter` interface
that `httpapi` declares, and `cmd/mdm` wires it (the same shape as
`Transcoder`).

**Rationale**: ffmpeg's `srt` demuxer does not detect the encoding itself
(`-sub_charenc` needs a build with iconv, which the bundled image and users'
ffmpeg builds do not share), so detection is needed in Go anyway. What remains
after detection is a small text rewrite of the number lines and timing lines;
there is no reason to start a process per request. Pure Go lets every input be
tested in a table without `ffmpeg`. It sits in `internal/media` because, like
`fmp4.go`, it handles a media format, and that adds no import between sibling
packages.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Start `ffmpeg -i x.srt -f webvtt -` per request | Rejected: see the rationale. |
| A new `internal/subtitles` package | Rejected: adds one more interface and wiring and is no different from one file in `internal/media`. |
| Put it in `internal/domain` | Rejected: brings `golang.org/x/text/encoding` into domain. Domain holds values and rules; decoding bytes is an adapter's job. |

## R-5: Character encoding is decided by BOM, then UTF-8 validity, then Shift_JIS

**Decision**: The leading bytes decide first. `EF BB BF` is UTF-8 and the BOM
is removed. `FF FE` and `FE FF` are UTF-16 (LE and BE), decoded with
`golang.org/x/text/encoding/unicode`. Without a BOM, the choice between the two
candidates UTF-8 and Shift_JIS (`golang.org/x/text/encoding/japanese`) is made
in this order:

1. If `utf8.Valid` holds and every decoded character belongs to a script used
   in subtitles (the Unicode script is one of `Common`, `Inherited`, `Latin`,
   `Greek`, `Cyrillic`, `Hebrew`, `Arabic`, `Thai`, `Hangul`, `Han`,
   `Hiragana`, `Katakana`, `Bopomofo`), UTF-8.
2. Otherwise, if the bytes decode as Shift_JIS (the output has no `U+FFFD` and
   no C1 control character `U+0080`–`U+009F`), Shift_JIS.
3. Otherwise, if `utf8.Valid` holds, UTF-8 (a UTF-8 subtitle in a script not on
   the list above).
4. Otherwise the file is treated as broken (R-7).

The detection is a pure function inside `internal/media` and looks at the bytes
of the whole file once.

**Rationale**: The three BOMs are decided uniquely by the leading bytes. UTF-8
and Shift_JIS without a BOM cannot be told apart uniquely: a Shift_JIS byte
sequence can also be valid UTF-8. For example, `E0 A1 A1` is `爍｡` in Shift_JIS
and `U+0861` (`ࡡ` in Syriac Supplement) in UTF-8. Reading such a sequence as
UTF-8 gives characters in a script that practically never appears in Japanese
subtitles, so the script check in step 1 discards UTF-8 and passes the file to
Shift_JIS. Ordinary UTF-8 subtitles in Japanese, English, Korean and similar
languages are decided in step 1 and never reach the Shift_JIS decoder. The
`golang.org/x/text` Shift_JIS decoder does not return an error for unreadable
sequences; it emits `U+FFFD` and passes `0x80` through as `U+0080`, so
"decodes" is checked by the absence of those characters in the output.

Shift_JIS sequences that are also valid UTF-8 within the scripts of step 1
(such as hiragana or kanji) still remain, and in that case UTF-8 is chosen. For
a whole BOM-less Shift_JIS file to pass `utf8.Valid`, every non-ASCII sequence
would have to happen to do so, which is unlikely in real Japanese subtitles.
The remaining confusion is checked with a real Shift_JIS file in the
quickstart. `golang.org/x/text` is already a dependency (the NFC normalization
in `internal/mediafs`). The `unicode` script tables are in the standard library.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Add an encoding-detection library | Rejected: adds a dependency, and there is no need to guess beyond the four encodings of requirement 4. |
| Decide UTF-8 from `utf8.Valid` alone | Rejected: shows Shift_JIS such as `E0 A1 A1` above garbled. |
| Score both decodings for how Japanese they look | Rejected: the scoring is arbitrary and its boundaries are hard to pin in a table test. |
| Also accept UTF-16 without a BOM | Rejected: requirement 4 covers only UTF-16 with a BOM, and without one it becomes guessing from sequences mixed with 0x00. |

## R-6: Live transcode timing: the server returns WebVTT shifted by `offsetMs`

**Decision**: The subtitle fetch route takes `offsetMs` (default 0) and
subtracts that value from the time of every cue. A cue whose end time becomes 0
or less is dropped, and a cue whose start time becomes negative starts at 0
([contracts/subtitles-api.md, `GET /api/videos/{id}/subtitles/{file}`](contracts/subtitles-api.md#get-apivideosidsubtitlesfile)).
Each time the player learns which time in the original video the playback
timeline's 0 is (the offset in `liveOffset.ts`), it reattaches the subtitle
tracks with that value as `offsetMs` in the URL. The value is 0 for direct
playback; for live transcode it is the actual start position from the
`transcode-start` report (the requested position when the report returns 404).
While the report is pending, no track is attached; tracks are attached once the
offset is settled.

**Rationale**: video.js subtitle display (emulated or native) picks cues by the
`currentTime` of the `<video>` element (the transcode output's timeline), and
the offset that the `liveOffset.ts` shim adds does not reach it. The shift has
to happen either on the server or by rebuilding cues in the browser. On the
server it is one pure function that Go tests can cover in a table. Rebuilding
in the browser needs a WebVTT parser, and the vtt.js bundled with video.js is
reachable only as a global variable. Subtitle files are small, and
reattachment happens only when a seek restarts the transcode (which already
takes several seconds). The parent Issue's edge case "do not show shifted
subtitles while the start position is unknown" is met by not attaching tracks
until the report settles.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Fetch and parse the VTT in the browser, shift the `VTTCue` times and add them with `addTextTrack` | Rejected: the parser problem, and every cue is rebuilt on every seek. |
| Rewrite the cues of the `TextTrack` in the shim | Rejected: requires changing the `activeCues` computation inside video.js, and has no effect on native tracks (Safari). |

## R-7: Broken files appear in the list and return 404 on fetch

**Decision**: The list (R-1) is built from file names and sizes only, without
reading content. The fetch route reads and converts. When the file cannot be
decoded (R-5), no SRT cue can be read, the WebVTT header is missing, the file
is empty or it exceeds the limit, the route returns 404 with `reason`
`subtitle_unavailable` and logs the cause at `Warn` on the server. The browser
fails to load that track and shows nothing.

**Rationale**: The parent Issue's edge case accepts either "not shown in the
menu" or "nothing appears when selected". Reading and converting every
subtitle on each list would read subtitles nobody uses every time the playback
screen opens. A subtitle that fails to fetch only puts the browser's `<track>`
into `error`; playback and the other subtitles continue.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Read on list and drop broken files | Rejected: see the rationale. |
| Return only the readable cues of a broken SRT | Adopted in this form: the readable cues are returned, and only an SRT with no readable cue gets 404. |

## R-8: SRT format variations: normalize only timing lines, pass cue text through

**Decision**: The conversion normalizes line endings to LF, removes the BOM,
adds the `WEBVTT` header, removes number lines (a line of digits only followed
by a timing line), changes `,` in times to `.` (`.` stays), and normalizes
variable digit counts for hours, minutes and seconds (`0:01:02,5`) to
`00:01:02.500`. A cue whose timing line cannot be read is dropped. Cue text,
including tags such as `<i>`, `<b>`, `<font …>` and `{\an8}`, passes through
unchanged.

**Rationale**: WebVTT cue text parsing in the browser discards unknown tags and
keeps the text inside them. Leaving that to the browser means the server keeps
no tag table, and without a table it does not break on new tags. The WebVTT
grammar for timing lines is strict (`hh:mm:ss.ttt`, `.` as the separator), so
only those lines are normalized.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Strip all tags to plain text | Rejected: not a requirement, and loses meaning such as italics. |
| Map `<font color>` to `<c.color>` | Rejected: needs CSS for WebVTT `::cue(c.color)`, and the parent Issue puts reproducing styling out of scope. |

## R-9: The subtitle selection is stored in `web/src/preferences` like the volume

**Decision**: `web/src/preferences/subtitlePreference.ts` reads and writes
`{ enabled: boolean, label: string }` (`""` for an unlabelled subtitle) under
the `localStorage` key `vv.subtitles.v1`, with total functions in the same
"default when unreadable, carry on when unwritable" form as
`playbackVolume.ts`. The default is `{ enabled: false, label: "" }`. The value
is written only when the user changes the selection with the menu or the `c`
key. It is not written when tracks are reattached because the offset changed
(R-6), nor when moving to a video without a matching label turns subtitles off.

**Rationale**: Requirement 7 of the parent Issue decides "per browser, like
the remembered volume". Clearing the stored value when a video without a match
turns subtitles off would leave subtitles off on the next video that has `ja`,
breaking requirement 7's "turned on automatically when a subtitle with the
same label exists".

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| The video.js `textTrackSettings` storage | Rejected: it stores display settings, not which track was chosen, and it also shows a settings dialog, so `textTrackSettings: false` is set. |
| Store on the server | Rejected: guest playback would need it too, and requirement 7 decides per browser. |

## R-10: Subtitle button and menu use video.js `SubsCapsButton`

**Decision**: `subsCapsButton` goes into the control bar's `children` before
the playback speed, and `textTrackSettings: false` hides the subtitle settings
item. Tracks are added with
`player.addRemoteTextTrack({ kind: "subtitles", src, label, default: false }, true)`.
The menu strings (`Subtitles`, `subtitles off`, `captions off` and others) are
replaced from the catalog by `playerDictionary()`, and the button gets
`withKey(…, "C")` and `aria-keyshortcuts="C"`. An unlabelled subtitle is shown
with the catalog name `t.player.subtitles.default`. Subtitle rendering is left
to video.js's `vjs-text-track-display`; only the bottom margin while the
control bar is visible (`bottom` under `vjs-user-active`) is matched in
`index.css` to the control bar height (including the 2em progress bar).

**Rationale**: `SubsCapsButton` hides itself when there is no subtitle track
(requirement 5), has an off item (requirement 5), and is the same component as
the existing playback speed menu, so it gets the same spacing and text size
(the parent Issue's UI quality). A menu built in React would have to rebuild
all of that and also keep the track `mode` in sync with the display itself.

**Alternatives considered**:

| Option | Verdict |
| --- | --- |
| Build the menu with a Radix Popover like `TranscodeIndicator` | Rejected: see the rationale. |
| Show separate `captionsButton` and `subtitlesButton` | Rejected: only `kind: "subtitles"` is used, so one button is enough. |

## R-11: The `c` key is added to `keyboard.ts`; the player decides the toggle

**Decision**: `shortcutFor` maps `c` and `C` to `"subtitles"`, and
`PlayerControls` gets `toggleSubtitles()`. The `VideoPlayer` implementation
does nothing when there is no track. When a track is showing, it sets every
track to `disabled` and stores "off". Otherwise it shows the track that matches
the stored label, or else the first track in the menu, and stores the choice.

**Rationale**: The key uses the same path as the existing keys (Space, F, M, 0,
Esc), so the rule that keys do nothing inside input fields and menus applies
unchanged. "The last chosen subtitle" is the R-9 stored value itself.

**Alternatives considered**: Enabling video.js `hotkeys` was rejected: the
existing design captures keys for the whole screen (Structural Decisions 9 in
specs/012-video-detail-ia).
