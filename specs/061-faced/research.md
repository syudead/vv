# Research: Recognizing people by their faces

Inherited decisions: [ARCHITECTURE.md](../../ARCHITECTURE.md) (layers,
store roles, events after commit, user data survives a rebuild),
[people.md](../../docs/design-docs/people.md) (people, faces, merge),
[activity.md](../../docs/design-docs/activity.md) (entries, pipelines, undo
dispatch), [auto-tagging.md](../../docs/design-docs/auto-tagging.md) (the
per-video queue and settings this feature mirrors) and
[windows-app.md](../../docs/design-docs/windows-app.md) (data folder,
`CREATE_NO_WINDOW`). The detector, the recognizer and the pipeline constants
come from the POC in PR #1393 (`scripts/facepoc`). This file records only the
decisions this feature adds.

## R-1: Inference runs in a helper process, never inside `mdm`

**Decision**: A separate Go program, `mdm-face`, loads onnxruntime through
`purego` and answers detection and embedding requests over its standard input
and output; `mdm` starts it as a child process and stays a static binary.

| Option | Alpine image | Crash in native code | Verdict |
| --- | --- | --- | --- |
| **Helper process started through the bundle's loader** | Runs: measured on `alpine:3.24` with the POC binary, Debian trixie's glibc and onnxruntime 1.22.0 (46 faces detected and embedded) | Ends the helper; the video is marked failed | Chosen |
| `purego` inside `mdm` | `mdm` no longer starts: a `purego` build gets the interpreter `/lib64/ld-linux-x86-64.so.2` and needs `libc.so.6`, and `exec` fails with `no such file or directory` on Alpine (measured) | Ends the server | Rejected: breaks every Docker user, including those who never enable the feature |
| Move the image to a glibc base (Debian) | Runs | Ends the server | Rejected: every user's image changes for an optional feature (requirement 1), and the hardware encoders are set up for Alpine's `ffmpeg` |
| `gcompat` on Alpine with glibc onnxruntime | Unverified: onnxruntime needs `libstdc++` built for glibc | Ends the server | Rejected: no evidence it runs, and `mdm` would still be dynamic |
| Inference in pure Go | Runs | None | Rejected: the convolutions would have to be written and kept equal to onnxruntime, at a speed already only 0.27 times the video length with onnxruntime |

**Rationale**: The process boundary keeps the dynamic loader, the C++
runtime and their crashes out of the server, and on Linux the helper runs
under the glibc it brings, so the image does not change. A request carries one
BGR frame or one aligned image; the reply carries the detections, their five
landmarks and their vectors. `mdm` keeps the tracking, the quality gate and
the clustering ([R-4](#r-4-main-people-are-found-per-video-with-the-pocs-rules)),
so the helper holds no rule.

The protocol is length-prefixed binary messages, starting with a handshake
that names the protocol version and the loaded models; a version mismatch
fails the download state, not the analysis.

## R-2: The face runtime is a pinned bundle that vv publishes

**Decision**: CI builds one face runtime archive per platform (`linux-amd64`,
`linux-arm64`, `windows-amd64`) and publishes it under a `face-runtime-v<N>`
GitHub Release; `mdm` pins each archive's URL and SHA-256 in code and downloads
it, with the selected models, only when the owner enables the feature.

| Archive contents | Linux | Windows |
| --- | --- | --- |
| `mdm-face` | Built with `CGO_ENABLED=0` | `mdm-face.exe` |
| onnxruntime 1.22.0 | `libonnxruntime.so` from Microsoft's release | `onnxruntime.dll` from Microsoft's release |
| C runtime | `ld-linux-*.so`, `libc`, `libm`, `libdl`, `libpthread`, `librt`, `libstdc++`, `libgcc_s` from Debian trixie (about 6 MB) | None |

| Option | Verdict |
| --- | --- |
| **vv's own release, pinned by hash** | Chosen |
| Ship the runtime in the image and the zip | Rejected: requirement 1; about 26 MB for every user |
| Download onnxruntime from Microsoft and the C runtime from Debian's packages at runtime | Rejected: `mdm-face` still has to come from vv, and unpacking `.deb` files at runtime adds a format the server never needs otherwise |
| Release the runtime with every vv version | Rejected: the runtime changes rarely, and nightly and development builds would have nothing to download |

**Rationale**: The archive changes only when onnxruntime or the protocol
changes, so a separate tag keeps nightly and tagged builds on the same file,
and the hash in code makes a replaced asset fail instead of run. Models come
from their upstream Hugging Face URLs, also pinned by SHA-256. Files go to
`<data>/face/runtime-v<N>/` and `<data>/face/models/`, written under `.tmp` and
renamed when the hash matches. Downloads use the process's proxy environment
(`HTTPS_PROXY`), not the plugin proxy, which is chosen for plugin traffic only.
On Linux, `mdm` starts `ld-linux-*.so --library-path <runtime>/lib mdm-face`;
on Windows it starts `mdm-face.exe` with `CREATE_NO_WINDOW`, as `ffmpeg` is
started. Other platforms report the feature as unavailable.

## R-3: Two recognition models, one detector

**Decision**: YuNet 2023mar detects faces for every choice; the recognizer is
SFace 2021dec int8 by default, or ArcFace r50 (InsightFace `buffalo_l`), which
the settings screen marks as licensed for non-commercial use only.

| Recognizer | Size | Vector | LFW accuracy | License | Verdict |
| --- | --- | --- | --- | --- | --- |
| **SFace int8** | 9.9 MB | 128 | 99.82% (Go POC) | Apache-2.0 | Default |
| **ArcFace r50** | 174 MB | 512 | 99.97% | Non-commercial research | Selectable, labelled |
| SFace fp32 | 37 MB | 128 | 99.86% | Apache-2.0 | Rejected: 0.04 points for four times the download |

**Rationale**: Both recognizers take the same five-point 112 × 112 alignment,
so only the input normalization and the vector length differ per model, and
both live in one model table in code. The detector stays the same, so a
model switch never changes which faces are found.

## R-4: Main people are found per video with the POC's rules

**Decision**: Each video is analysed alone: frames at 2 fps fitted into
1280 × 1280, detection, tracks, the quality gate, average-linkage clustering
at the model's cluster threshold, and the cutoff of 8 s of good faces in 2 or
more tracks, all as in the POC; the rules are pure functions in
`internal/domain`.

| Option | Verdict |
| --- | --- |
| **Per video, then matched to stored people** | Chosen |
| Cluster the whole library together, as the POC did | Rejected: every import would re-cluster every video, and stable person ids could change |

**Rationale**: A video's result is final when its analysis ends, so the links
it makes can be recorded and undone as one activity entry. The constants stay
in code, as the POC measured them together; a setting would let an owner break
the balance between the quality gate and the cutoff without seeing it.

## R-5: A main person is compared with every stored vector

**Decision**: A person's vectors are the centroid of each appearance linked
to them and the vector of their face image; a main person's similarity to a
person is the highest cosine similarity among those vectors, computed in
memory over all people.

| Option | Verdict |
| --- | --- |
| **Every vector, in memory** | Chosen |
| One averaged vector per person | Rejected: the POC found the same actor across films with different hair; an average blurs those looks |
| A nearest-neighbour index | Rejected: 10,000 vectors of 512 floats are 20 MB and a full scan takes milliseconds |

## R-6: Similarity thresholds are constants per recognizer

**Decision**: Each recognizer has three thresholds in its model table:
`candidate` ≤ similarity < `link` makes a candidate, ≥ `link` links an
appearance, and ≥ `merge` merges a provisional person into a person with a
new face image.

| Recognizer | `candidate` | `link` | `merge` | Source |
| --- | --- | --- | --- | --- |
| SFace int8 | 0.363 | 0.45 | 0.55 | Upstream same-person threshold; POC at 0.45: 3.2% of unknown people named; at 0.55: 0.1% |
| ArcFace r50 | Measured | Measured | Measured | The unit that adds the model measures them with the POC's LFW protocol at the same false-naming rates as SFace |

| Option | Verdict |
| --- | --- |
| **Constants per recognizer** | Chosen |
| Owner settings | Rejected: the values differ per model, and the owner has no data to set them |

## R-7: A provisional person is a person without a canonical name

**Decision**: Face analysis creates people with no `people_names` row, and a
person is provisional exactly when it has no canonical name; giving it a name
makes it an ordinary person in the same write.

| Option | Verdict |
| --- | --- |
| **No canonical name** | Chosen |
| A `provisional` column beside the name | Rejected: two facts that can disagree (a named provisional person) |
| A generated placeholder name such as `Unknown 12` | Rejected: a stored name cannot follow the screen's language, it matches name searches, and a merge would add it as an alias |

**Rationale**: The API keeps `name` required and returns `""` with
`provisional: true`, so clients that do not know the flag still parse the
response; the screens show the face and a translated label instead. A name
sort puts provisional people last. A merge into a provisional target takes the
first named source's canonical name as its own, so a merge never leaves names
without a canonical one.

## R-8: Face data is keyed by content key and survives a model switch by its spans

**Decision**: An appearance (one main person in one video) is stored by
content key with its tracks (start, end, mean face centre and mean face
width), its centroid, its crop, its recognizer and the person it is linked
to; re-analysis carries an old appearance over to the one new appearance that
holds more than half of its matched track seconds, where a new track matches
an old one when their times overlap and their centres lie no farther apart
than the smaller of the two tracks' mean face widths. The carried-over row
keeps its id, its person, its dropped mark and its crop, and takes the new
tracks, counts, recognizer and, unless its person was deleted, centroid; only
a new appearance that nothing
carries over to is matched by vector.

| Option | Verdict |
| --- | --- |
| **Carry over by tracks: time and position, one to one, keeping the person** | Chosen |
| Time overlap of spans alone | Rejected: two people on screen together overlap each other's spans, so their people could swap |
| Match the new vectors against the old ones | Rejected: vectors of two recognizers cannot be compared, so a model switch would create a second provisional person for everyone |
| Carry the row over but match its new centroid by vector again | Rejected: right after a switch, the other videos' centroids still belong to the old recognizer, and a new match would move links the owner kept or confirmed, which the parent Issue's model-switch edge case keeps |
| A centre distance of a fixed fraction of the frame | Rejected: a close-up and a wide shot need different distances; the face width scales with the shot |

**Rationale**: The detector is the same for every recognizer
([R-3](#r-3-two-recognition-models-one-detector)), so the same video yields
the same faces at the same times and places; position separates people who
share the screen. A carry-over is one to one: when two old appearances claim
the same new one, or none has a majority, nothing is carried over and the new
appearance is matched by vector as a first analysis is. The smaller width
keeps a small face beside a large one from claiming the large one's track.
Keeping the id lets an activity entry written before the re-analysis still
undo the link
([R-10](#r-10-an-automatic-link-is-a-performer-credit-and-removing-it-drops-the-appearance)).
A wrong link is corrected by the owner, by removing the performer or undoing
the link, which drops the appearance; a re-analysis does not move a
carried-over link. The crop stays as written, so its URL can be cached
([contract](contracts/face-recognition-api.md#get-apiface-appearancesidcrop)),
and a row whose person was deleted gets neither a centroid nor a crop back
([R-15](#r-15-a-persons-face-data-is-deleted-with-the-person)).
Content key, not `videos.id`, follows the rebuild invariant in
[ARCHITECTURE.md](../../ARCHITECTURE.md#cross-cutting-invariants).

## R-9: Face analysis has its own queue, and turning it off stops the running video

**Decision**: `face_queue` holds one row per content key, queued in the
transaction that records the seek preview stage's final result, as
auto-tagging does; one worker analyses one video at a time and cancels the
running analysis when the feature is turned off, leaving the row `queued`.

| Option | Verdict |
| --- | --- |
| **Own queue, as `auto_tag_queue`** | Chosen |
| A job kind in `jobs` | Rejected: import progress counts jobs, and an optional analysis at 0.27 times the video length would hold the import open |
| Let the running video finish, as auto-tagging does | Rejected: the parent Issue's edge case asks to analyse that video again when the feature comes back |

**Rationale**: The entry joins the import pipeline as `auto_tag` does
([activity.md](../../docs/design-docs/activity.md#a-pipeline-groups-one-import-run)).
onnxruntime gets half of the CPU cores (at least one) for its threads, so
playback and live transcoding keep the rest.

## R-10: An automatic link is a performer credit, and removing it drops the appearance

**Decision**: Linking writes a `performer` row in `video_work_people` at the
video's user key; when the owner removes that performer from the video, undoes
the link or deletes the person, the appearance is marked dropped, and a dropped
appearance is never linked again or counted among the person's vectors.

| Option | Verdict |
| --- | --- |
| **Performer row, dropped appearance** | Chosen |
| A separate face-link table beside credits | Rejected: the person page, the filters and smart folders read credits only; a second table would need every reader to change |
| A `source` column on `video_work_people` | Rejected: the details editor and the plugin apply replace the rows, so the source would be lost on every edit |
| A table of rejected person and video pairs | Rejected: a deleted person leaves no id to reject, and the pair would not follow a re-analysis the way tracks do |

**Rationale**: Whether a credit came from a face is answered by the
appearances linked to it, not by the credit row. Re-analysis carries the
dropped mark over by span
([R-8](#r-8-face-data-is-keyed-by-content-key-and-survives-a-model-switch-by-its-spans)),
so a removed link comes back neither as the same person nor as a new
provisional one.

## R-11: An automatic merge keeps what it moved, so undo can move it back

**Decision**: An automatic merge records the source person's row, profile,
favorite, appearances, candidates and smart-rule references with its
`face_merge` activity entry, and each credit it moved (and whether the target
already had it) in a row of its own; undo re-creates the source under its old id and moves back only what
the merge moved and the owner has not changed since.

| Option | Verdict |
| --- | --- |
| **Snapshot of what moved** | Chosen |
| Keep merged sources as hidden rows | Rejected: every read of people and credits would have to skip them |
| Detect the owner's credit edits by comparing the credits at undo | Rejected: a credit the owner removed and added again looks unchanged |
| Make every merge undoable, the owner's too | Rejected: the parent Issue asks for undo of automatic decisions, and owner merges have no activity entry today |

**Rationale**: SQLite's `AUTOINCREMENT` never reuses an id, so re-inserting
the old id is safe and old links to `/people/{id}` work again. The owner's
later changes cannot be read from activity entry ids, as auto-tag undo reads
them: editing a video's credits and editing a person write no activity entry.
Undo therefore puts back a field of the target only while it still holds the
value the merge gave it, and moves back a credit only while it is unmarked:
every work-information write (the details editor, the library selection, the
plugin apply and `/api/v1`, all through `WorkInfoStore`) marks, in its
transaction, the recorded credits it removes from the target. A credit the
owner removes and adds again is therefore the owner's, and stays.

## R-12: A face image is embedded after it is stored

**Decision**: Storing or replacing a face publishes `PersonImageChanged`
after commit, and naming a provisional person does the same for its face; the
face worker embeds the face between videos, and when the person is named and
exactly one provisional person reaches `merge`, merges it automatically,
otherwise it records candidates. A provisional person's own face starts no
merge, so provisional people never merge into each other on their own. A manual upload while the runtime is ready checks the image
first and is refused when it has no good face or more than one.

| Option | Verdict |
| --- | --- |
| **Event, then the face worker** | Chosen |
| Embed inside the upload request | Rejected for plugin and merge faces: they are stored after an apply or a merge commits, and the helper may be busy with a video |
| Check plugin faces too | Rejected: the parent Issue asks for it only on the owner's upload; a plugin face without a good face simply gets no vector |

## R-13: The cast rule merges only one-to-one pairs

**Decision**: After a video's work information changes or its analysis ends,
when exactly one performer of that video has no appearance in it and exactly
one provisional person is linked to it by face, the provisional person is
merged into that performer as a `face_merge`.

**Rationale**: The parent Issue's requirement 9 needs no similarity, because
the performer has no face to compare with; the POC measured 0.5% false naming
at 0.363 with five candidates, and one-to-one is stricter still. A dismissed
candidate pair is never merged this way.

## R-14: Candidates are person pairs with a dismissed state

**Decision**: `face_candidates` stores a provisional person, a person, the
similarity and whether the owner dismissed the pair; confirming is the
ordinary merge of the provisional person into the person, and a merge moves the
source's pairs to the target and drops pairs that would point at one person.

| Option | Verdict |
| --- | --- |
| **Pairs of people** | Chosen |
| Pairs of appearances | Rejected: the person page offers people, and one person's many appearances would repeat one choice |

## R-15: A person's face data is deleted with the person

**Decision**: Deleting a person deletes, in the same transaction, the face
image, its vector, and the centroids and crops of the person's appearances;
the appearances keep only their tracks with the dropped mark, so a later
analysis does not create the same face again. There is no separate "Delete
face data" operation.

| Option | Verdict |
| --- | --- |
| **Face data goes with its person** | Chosen |
| An operation that deletes all face data | Rejected: the maintainer removed it from the parent Issue; on a self-hosted library the owner deletes people, and the next analysis would compute the vectors again anyway |
| Delete the appearances too | Rejected: the next analysis would find the same face and create the deleted provisional person again |

**Rationale**: Tracks hold times and positions, not a face, and are what lets
re-analysis recognize a dropped appearance
([R-8](#r-8-face-data-is-keyed-by-content-key-and-survives-a-model-switch-by-its-spans)).

## R-16: Download progress is read from the settings resource

**Decision**: `GET /api/settings/face-recognition` carries the runtime state
(`absent`, `downloading` with bytes done and total, `ready`, `failed` with a
reason), and the settings section reads it every second while downloading.

| Option | Verdict |
| --- | --- |
| **Polling the settings resource** | Chosen |
| A new SSE event | Rejected: one screen watches a download that happens once; the auto-tagging section already polls its counts the same way |

## R-17: A generation number fences results computed before a recognizer switch

**Decision**: The settings row `face.generation` increases on a recognizer
switch, in its transaction; every analysis and embedding records the
generation it started under, and the store refuses to write a result whose
generation is no longer current. The switch also cancels the running
analysis.

| Option | Verdict |
| --- | --- |
| **Generation checked in the result's transaction** | Chosen |
| Cancel the worker and wait for it before switching | Rejected: an embedding queued by an event after the wait would still write; the wait alone does not close the race |
| A lock held for the whole analysis | Rejected: an analysis takes minutes, and the settings request would wait for it |

**Rationale**: The check sits in the same transaction as the write, so a
vector of the old recognizer can never be stored after the switch, where it
would be compared with vectors it cannot be compared with. A deleted person
needs no generation: the result's transaction re-reads the people it links.

## R-18: A provisional person's face is the best crop of its first appearance

**Decision**: Each appearance keeps a crop of its best good face, the face
box doubled around its centre and cut from the 1280 × 1280 frame without
enlarging; a provisional person created by analysis gets that crop as its
face image, stored as an ordinary face in `person_images`.

| Option | Verdict |
| --- | --- |
| **The crop becomes the person's face** | Chosen |
| Keep crops only on the appearances | Rejected: the people grid, the cast chips and the candidates show a person's face image, so a provisional person would show no face where the owner judges it |
| A 112 × 112 aligned crop | Rejected: it is the model's input, cut tight to the eyes and mouth; too small and too tight for a person's portrait |

**Rationale**: The owner judges a provisional person and a candidate by
looking at the face, and the screens already show `person_images`
([people.md](../../docs/design-docs/people.md#a-face-is-one-fitted-jpeg-in-sqlite)).
A merge into a person without a face then brings the crop along, as every
merge brings a face.
