# Bundled versions keep every side's data

Bundling videos as versions merges every side's user data into the new
bundle's key, so the representative decides only which file is shown and
played ([`internal/store/versions.go`](../../internal/store/versions.go)).
Duplicates recommends the representative from the files alone and shows what
tells the two files apart.

## Every side's user data reaches the bundle

`applyBundle` creates a new user key and merges each side's key into it, the
representative's first. A side already in a bundle contributes its bundle's
key, never its members' own keys, which keep what they held before they joined.

| Data | Merged as |
| --- | --- |
| Manual tags, favourites | The union |
| Publication | Public when either side is public: both files are the same video |
| Playback position | The most recently updated side's |
| Work information (code, description, dates, URLs, IDs, people, organisations) | The first side in order that has any, the representative's first; one record across its tables, since mixing two would describe neither work |
| Auto-tagging provenance | Only for the tag rows that side added, so undoing an automatic tag never removes the owner's own row on the other side |

Display names and applied thumbnails belong to each file's content key, not
the user key, so the bundle shows the representative's. Undo of a drag-organize
bundle and an automatic bundle's undo split the merged data as before:
leaving members of two or more copy the bundle's data, and a single member gets
its own key back.

| Rejected | Why |
| --- | --- |
| The representative's data only (before) | The owner had to pick the file with the tags rather than the better file, or lose the other side's tags and position |
| Merging work information field by field | Two different works' dates and people would mix |

## The owner's choice of representative stands

`video_bundles.representative_by_owner` marks a representative the owner
picked: in the bundle dialog, by dragging onto a video, or with Make
representative. Automatic bundling keeps such a representative and otherwise
chooses by the files (`domain.ChooseVersionRepresentative`). Bundles that
existed before the column count as picked, since most were bundled by hand.
When the owner-picked representative leaves the bundle, the next one is not
owner-picked.

## Duplicates recommends by the files

`GET /api/version-candidates` returns `recommendation` for each pair from
`domain.RecommendVersionRepresentative`, with the first property that told the
files apart:

| Order | Prefer | Reason |
| --- | --- | --- |
| 1 | Plays in the browser without converting | `playable` |
| 2 | Has audio | `audio` |
| 3 | Its first audio track is as long as its video, within a second and 1% | `audio_sync` |
| 4 | More pixels | `resolution` |
| 5 | Higher video bit rate, or file bytes per second when either is unknown | `bitrate` |
| 6 | Lower ID | `older` |

A track that ends early or runs long is what a cut, a dropped stretch or
drifting sync leaves. The start gap between the audio and the video is shown
but never counts, since players align the streams by it. The video bit rate,
not the file size, decides, so lossless audio or attachments do not make a
file look better.

The bundle dialog opened from a pair selects the recommended video and says
that both sides' data is kept. Each file also lists what tells it apart, from
its media information ([media-information.md](media-information.md#items-for-comparing-files)):
needing conversion to play, encryption, a size at least 1.25 times the other's
without more pixels, audio taking 30% or more of the file, audio starting at
least half a second off the video, and audio ending a second or more off the
video ([`representativeHints.ts`](../../web/src/versions/representativeHints.ts)).
