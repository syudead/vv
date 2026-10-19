# Face recognition

vv finds the main people of each video by face and matches them to the
people in the library. Faces are detected and embedded by a helper process,
`mdm-face` ([`cmd/mdm-face`](../../cmd/mdm-face)), that runs onnxruntime
outside `mdm` and arrives in a face runtime archive that vv publishes; the
rules that turn its faces into people are pure functions in
[`internal/domain`](../../internal/domain/face_main_people.go).

The diagram shows the parts that carry a frame to its face vectors.

```mermaid
flowchart LR
  mdm[mdm, static] -->|faceproto request| helper[mdm-face]
  helper -->|purego| ort[onnxruntime]
  ort --> yunet[YuNet detector]
  ort --> rec[Recognizer]
  helper -->|faces and vectors| mdm
  release[face-runtime release] -->|archive| helper
```

## Inference runs in a helper process

`mdm` never loads native code: inference runs in `mdm-face`, which loads
onnxruntime through `purego` ([`internal/faceinfer`](../../internal/faceinfer)),
and only `cmd/mdm-face` may import `purego` or `internal/faceinfer` (the
`only-the-face-helper-loads-native-code` rule in
[`.golangci.yml`](../../.golangci.yml)).

A `purego` binary asks for glibc's dynamic loader (`/lib64/ld-linux-x86-64.so.2`)
in an `INTERP` program header, and such a binary does not start in the
`alpine:3.24` image, so `mdm` inside it would fail for every Docker user. The
helper's own crashes in native code end the helper, not the server.
`task static-binary-check` builds the Linux `mdm` for `amd64` and `arm64` and
fails when either has an `INTERP` header; CI runs it on every pull request.

| Rejected | Why |
| --- | --- |
| `purego` inside `mdm` | `mdm` no longer starts on Alpine |
| A glibc base image | Every user's image changes for an optional feature |
| Inference in pure Go | The convolutions would have to be written and kept equal to onnxruntime |

## The helper protocol

`mdm` and the helper exchange length-prefixed binary messages on the helper's
standard input and output ([`internal/faceproto`](../../internal/faceproto)),
and the helper holds no rule about which faces matter.

The sequence shows one helper's life.

```mermaid
sequenceDiagram
  participant M as mdm
  participant H as mdm-face
  H->>M: Hello (version, detector, recognizer, vector length)
  M->>H: DetectRequest (BGR frame)
  H->>M: Detections (box, 5 landmarks, score, vector per face)
  M->>H: EmbedRequest (112 x 112 aligned BGR face)
  H->>M: Vector
  M--xH: close standard input
```

| Message | Rule |
| --- | --- |
| `Hello` | The helper's first message; a different `Version` is an error on read, so a stale archive is reported instead of misread |
| `Error` | Replaces `Hello` when the models do not load, and answers a request that failed; the helper keeps serving after a failed request |
| `Detections` | Every face YuNet keeps (score 0.6, non-maximum suppression at IoU 0.3), each with its vector; tracking and the quality gate stay in `mdm` |

Each message is a little-endian `uint32` length, a kind byte and its payload,
at most 64 MiB. The helper exits when its input closes.

## Two recognizers share one detector

YuNet 2023mar detects faces for every recognizer; the recognizer is SFace
2021dec int8 (`sface_int8`) or ArcFace r50 from InsightFace `buffalo_l`
(`arcface_r50`).

| Recognizer | Input | Vector |
| --- | --- | --- |
| `sface_int8` | Planar RGB, 0..255 | 128 |
| `arcface_r50` | Planar RGB, (v − 127.5) / 127.5 | 512 |

Both take the same five-point alignment to a 112 × 112 template, so a switch
never changes which faces are found. The recognizer's input and output names
are read from the model, because exports of one model name them differently.
The frame path aligns without rounding to bytes, as the measured proof of
concept did: on LFW the Go SFace int8 pipeline reached 99.82% accuracy against
OpenCV's 99.86% with SFace fp32.

## The face runtime archive

CI builds one archive per platform from
[`scripts/build/face_runtime.go`](../../scripts/build/face_runtime.go)
(`go run ./scripts/build -face-runtime <platform>`), and the
[face-runtime workflow](../../.github/workflows/face-runtime.yml) publishes
them as a `face-runtime-v<N>` GitHub Release when the maintainer pushes that
tag.

| Archive | Contents |
| --- | --- |
| `face-runtime-linux-amd64.tar.gz`, `face-runtime-linux-arm64.tar.gz` | `mdm-face`; `lib/` with Microsoft's onnxruntime 1.22.0 and the loader, `libc`, `libm`, `libdl`, `libpthread`, `librt`, `libstdc++` and `libgcc_s` of a pinned `debian:trixie` image; their licenses |
| `face-runtime-windows-amd64.zip` | `mdm-face.exe`, `onnxruntime.dll`, its license |

On Linux the helper starts as `lib/ld-linux-*.so --library-path lib mdm-face`,
so it runs under the bundled glibc on any host, Alpine included. Each pull
request that changes the helper builds the three archives, and the Linux jobs
start each helper in `alpine:3.24` through that loader on one photo and expect
one face with a 128-value vector. A separate tag keeps nightly and tagged vv
builds on the same archive, which changes only with onnxruntime or the
protocol.

| Rejected | Why |
| --- | --- |
| The runtime in the image and the Windows zip | About 26 MB for every user, including those who never turn the feature on |
| onnxruntime and Debian packages fetched by `mdm` | `mdm-face` still has to come from vv, and unpacking `.deb` files adds a format the server needs nowhere else |
| A runtime per vv release | Nightly and development builds would have nothing to download |

## The runtime is downloaded only when the feature is on

`mdm` pins the URL and SHA-256 of each platform's runtime archive and of each
model in code
([`internal/faceruntime`](../../internal/faceruntime/catalog.go)), and
downloads them only when the owner turns face recognition on, or asks to
download again after a failure. The models come from their upstream Hugging
Face repositories, each URL pinned to a commit.

| File | Source | Size |
| --- | --- | --- |
| Runtime archive | The `face-runtime-v1` release (`linux-amd64`, `linux-arm64`, `windows-amd64`) | 5.9 to 11.6 MB |
| YuNet 2023mar | `opencv/face_detection_yunet` (MIT) | 0.2 MB |
| SFace 2021dec int8 | `opencv/face_recognition_sface` (Apache-2.0) | 9.9 MB |
| ArcFace r50 | `immich-app/buffalo_l`, `recognition/model.onnx` (InsightFace, non-commercial) | 174 MB |

A recognizer needs the runtime, YuNet and its own model. Each file is written
under `<data>/face/.tmp/`, which a download empties first, and moved into
place only when its hash matches: the archive is unpacked into
`<data>/face/runtime-v<N>/` and must then hold exactly the files whose hashes
`mdm` pins for that archive, with the execute bit on the loader and `mdm-face`
on Linux, and the models go to `<data>/face/models/`. Downloads use the process's proxy environment
(`HTTPS_PROXY`), because the plugin proxy is chosen for plugin traffic only.

The download state is not stored. At startup `mdm` hashes the files against
the pins built into it, and `GET /api/settings/face-recognition` reports
the state of what the chosen recognizer needs; the settings page reads the
progress from the same resource while downloading.

A recognizer is `ready` only when its files match their pins and its helper
has greeted for those files in this process. Files that all match at startup
count as greeted, so startup starts no helper. `mdm` keeps the last helper
outcome of each recognizer separately in memory, so choosing another
recognizer leaves a `failed` one failed, and choosing it again starts its
helper again.

The diagram shows the states of one recognizer's files.

```mermaid
stateDiagram-v2
  [*] --> unsupported: no archive for this platform
  [*] --> absent: a file missing
  [*] --> ready: every file matches
  [*] --> failed: a file differs (hash_mismatch)
  absent --> downloading: turned on
  failed --> downloading: download again, turned on, or chosen again
  downloading --> ready: hashes match and the helper greets
  downloading --> failed: network, hash_mismatch, disk, helper_start
  downloading --> absent: turned off
```

| Event | Effect |
| --- | --- |
| Turning the feature on, or switching the recognizer while on | Downloads what the recognizer needs that is missing or differs; a download for the other recognizer is cancelled |
| Turning the feature off | Cancels a running download; the files already verified stay, and a recognizer whose helper had not greeted is `absent` until the next start probes it |
| Startup while on | Resumes a download only when files are missing; a file that differs stays `failed` until the owner downloads again |
| A download ends | The helper is started once: a `Hello` of another protocol version, or no `Hello`, is `helper_start` |
| Any helper start | The runtime and the recognizer's models are hashed against their pins again first; a helper whose files no longer match is not started and is `hash_mismatch` |
| Any download start, downloading again included | The files the recognizer needs are hashed again rather than taken from an earlier check, so a file deleted or changed since then is downloaded again |
| Any start of a recognizer whose helper has not greeted, with no file missing | The helper is started once again instead of the state turning `ready` unchecked: after a failure, a cancelled handshake, or switching back to a recognizer that failed |
| A recognizer switch | Increases `face.generation` in the settings' transaction |

On Linux `mdm` starts `runtime-v<N>/lib/<loader> --library-path
runtime-v<N>/lib runtime-v<N>/mdm-face`; on Windows it starts `mdm-face.exe`
with `CREATE_NO_WINDOW`, through the same command builder as `ffmpeg`. The
helper gets half of the CPU cores, at least one, for onnxruntime's threads, so
playback and live transcoding keep the rest. The client starts the helper on
its first request; a helper whose stream breaks fails that request and is
started again on the next one, and a cancelled request stops it.

| Rejected | Why |
| --- | --- |
| Storing the download state in the settings | It would disagree with the files after a crash or a deleted folder; the hashes are the state |
| Keeping the runtime archive to verify it at startup | It doubles the disk use; the per-file pins built into `mdm` verify the unpacked files |
| A hash manifest written into the runtime folder | Anyone who can write the folder could replace `mdm-face` and its hash together; the trust root must be the `mdm` binary |
| Starting the helper from a file descriptor or an immutable copy of the verified files | A short window between the check and the start remains, but a writer of the data folder as the service user can already change the database and settings |
| Downloading again at startup a file that differs | The owner would not see that a file broke; the settings page shows `failed` and offers the download |

## From faces to people

`mdm` turns the helper's faces into people with the rules below, one video at
a time. The diagram shows how one video's faces become links, candidates and merges.

```mermaid
flowchart LR
  frames[Faces of 2 fps frames] --> tracker[FaceTracker: tracks]
  tracker --> main[FindMainPeople: gate, clustering, cutoff]
  main --> plan[PlanFaceAppearances]
  stored[Stored appearances of the video] --> plan
  plan -->|carried over| keep[Keeps its person and dropped mark]
  plan -->|new| match[MatchFace]
  match --> link[Link]
  match --> prov[New provisional person and candidates]
  image[Face image of a person] --> imageRule[DecideFaceImageMerge]
  cast[Performers of a video] --> castRule[DecideCastMerge]
```

## Main people are found per video with the POC's constants

A video's main people are clusters of its face tracks with at least 8 seconds
of good faces in at least 2 tracks
([`face_main_people.go`](../../internal/domain/face_main_people.go)).

The constants were measured together in the face POC on 12 real videos, where
20 of 22 main people held one person only; they are code, not settings, because
the quality gate and the cutoff only work as a pair.

| Step | Rule |
| --- | --- |
| Sampling | 2 frames per second, fitted into 1280 × 1280 |
| Tracking | Faces under 48 px are skipped; a face continues the open track it is most similar to above `Track` when the boxes overlap (IoU over 0.05), or above `TrackFar` when they do not, and the most similar face and track pairs of a frame are joined first; a track closes after 2 missed frames and is kept with 3 or more faces |
| Quality gate | A good face is 64 px wide or more, scores 0.85 or more, and has its nose within 0.5 ± 0.22 of the way between the eyes |
| Track vector | The normalized mean of the good faces; a track with fewer than 2 good faces has none and is not clustered |
| Clustering | Average linkage over the track vectors, joining while the mean similarity reaches `Cluster` |
| Cutoff | Good seconds (good faces ÷ 2) at least 8, tracks at least 2 |

Each track keeps its start, end, mean centre and mean width, and a main person
keeps its best good face (largest box times score) for its crop.

| Rejected | Why |
| --- | --- |
| Clustering the whole library together | Every import would re-cluster every video, and person ids could change |

## Each recognizer has its own thresholds

Similarity thresholds are constants in the recognizer table
([`face_recognizer.go`](../../internal/domain/face_recognizer.go)), because
SFace and ArcFace vectors spread differently and the owner has no data to set
them.

| Threshold | Use | SFace int8 | ArcFace r50 |
| --- | --- | --- | --- |
| `Track` | Continue a track, boxes overlapping | 0.45 | 0.288 |
| `TrackFar` | Continue a track, boxes apart | 0.6 | 0.614 |
| `Cluster` | Join clusters of tracks | 0.45 | 0.288 |
| `Candidate` | Offer a pair to the owner | 0.363 | 0.205 |
| `Link` | Link a main person to a person | 0.45 | 0.288 |
| `Merge` | Merge a provisional person on a face image | 0.55 | 0.545 |

The SFace values are the upstream same-person threshold and the POC's. The
ArcFace values give the same error rates as SFace on LFW (13,233 images of
5,749 people), measured with the POC's protocol on vectors from the same
detection and alignment:

- **Late linking** (`Candidate`, `Link`, `Merge`): 70% of the people with two
  or more images are registered with one random image each; every other image
  is named by its most similar registered person when the similarity reaches
  the threshold. A correct naming is a registered person's image given their
  own name; a false naming is an unregistered person's image given any name.
  Rates are the mean of 5 random draws.
- **Pairs** (`Track`, `TrackFar`, `Cluster`): the share of all pairs of images
  of two different people at or above the threshold.

The measured rates, with the 4,950 registered and 7,107 unregistered images
of a draw and the 87,307,271 pairs of different people:

| Threshold | SFace int8: value, rate | ArcFace r50: value, rate |
| --- | --- | --- |
| `Candidate` | 0.363: 98.3% correct, 61.6% false | 0.205: 99.8% correct, 61.3% false |
| `Link` | 0.45: 96.0% correct, 3.43% false | 0.288: 99.7% correct, 3.42% false |
| `Merge` | 0.55: 83.9% correct, 0.17% false | 0.545: 90.4% correct, 0.17% false |
| `Track`, `Cluster` | 0.45: 4,482 pairs (0.0051%) | 0.288: 4,442 pairs |
| `TrackFar` | 0.6: 88 pairs (0.0001%) | 0.614: 87 pairs |

At equal false rates ArcFace names more people correctly, most of all at
`Merge`. An unregistered image named at `Candidate` is why that threshold only
offers a pair and never links.

## A main person is matched against every vector of a person

A person's vectors are the centroids of their appearances and the vector of
their face image, and a main person's similarity to them is the highest cosine
among those
([`MatchFace`](../../internal/domain/face_match.go)).

| Highest similarity | Result |
| --- | --- |
| `Link` or more | Linked to the most similar person |
| `Candidate` up to `Link` | A new provisional person, offered with each such person as a candidate |
| Below `Candidate` | A new provisional person |

An appearance the owner dropped (by removing or undoing its link, or deleting
its person) is not a vector of its person. The face POC found one actor across
films with different hair; an averaged vector per person would blur those
looks.

## A re-analysis carries appearances over by track time and position

A re-analysed video keeps each stored appearance by passing it to the one new
appearance holding more than half of its matched track seconds
([`CarryOverFaceAppearances`](../../internal/domain/face_match.go)); a
carried-over appearance keeps its person and dropped mark and is not matched by
vector again.

```mermaid
flowchart TD
  pair[Old track, new track] --> time{Times overlap?}
  time -->|no| none[No seconds]
  time -->|yes| dist{Centre distance at most the smaller mean width?}
  dist -->|no| none
  dist -->|yes| sec[Shared seconds count for the new appearance]
  sec --> major{One new appearance holds over half of the old one's seconds?}
  major -->|no| nothing[Not carried over]
  major -->|yes| claim{Another old appearance claims it too?}
  claim -->|yes| nothing
  claim -->|no| carry[Carried over]
```

The detector is the same for every recognizer, so a recognizer switch finds the
same faces at the same times and places, while vectors of two recognizers
cannot be compared. Position separates people on screen together, and the
smaller width keeps a small face beside a large one from claiming the large
one's track. A new appearance nothing is carried over to is matched by vector.

| Rejected | Why |
| --- | --- |
| Time overlap alone | Two people on screen together overlap each other's spans, so their people could swap |
| Matching the carried-over centroid again | After a switch the other videos' vectors still belong to the old recognizer, and links the owner kept would move |
| A fixed fraction of the frame as the distance | A close-up and a wide shot need different distances; the face width scales with the shot |

## A face image merges only one provisional person

When a named person's face image reaches `Merge` with exactly one provisional
person, that person is merged into them; two or more at `Merge`, and every
provisional person from `Candidate` up, are candidates
([`DecideFaceImageMerge`](../../internal/domain/face_match.go)). A provisional
person's own face merges nobody, so provisional people never merge into each
other on their own.

## The cast rule merges only one-to-one pairs

When exactly one performer of a video has no face linked in it and exactly one
provisional person is linked to it by face, the provisional person is merged
into that performer, unless the performer is provisional too or the owner
dismissed the pair
([`DecideCastMerge`](../../internal/domain/face_match.go)).

The performer has no face to compare with, so the rule relies on the
one-to-one pairing; in the POC, five candidates from a cast list named 0.5% of
unknown faces wrongly at 0.363, and one candidate is stricter still.

## One video's result is stored in one transaction

`FaceStore.Store` writes a video's analysis in one transaction
([`faces.go`](../../internal/store/faces.go)): it re-reads the stored
appearances and every person's vectors, applies `PlanFaceAppearances`, and
writes what the plan decides. Reading the people inside the transaction means a
person deleted during the analysis is never linked.

| Plan for a main person | What is written |
| --- | --- |
| Carried over | The row keeps its id, person, dropped mark and crop, and takes the new tracks, counts, recognizer and centroid; a row whose person was deleted keeps a null centroid |
| Linked to a person | A new appearance and a `performer` row at the video's user key |
| New | A provisional person (a `people` row without names) whose face is the crop, its candidate pairs, a new appearance and a `performer` row |

A stored appearance nothing is carried over to is deleted. The completed
`face_analysis` entry lists each link it made in
`activity_entry_face_links`, with `created_person` set when the run created the
person. Appearances are keyed by content key, so a content key that no video
has any more keeps its rows, and a returning file keeps its links.

| Rejected | Why |
| --- | --- |
| Matching in the worker, then writing | A person deleted or merged between the read and the write would be linked |
| Keeping crops only on appearances | The people grid, the cast chips and the candidates show `person_images`, so a provisional person would show no face |

## The face queue mirrors the auto-tagging queue

`face_queue` holds one row per content key in the states `queued`, `running`,
`done` and `failed`, and shares its queueing, claiming, failure and recovery
code with `auto_tag_queue`
([`content_queue.go`](../../internal/store/content_queue.go)). While
`face.enabled` is `true`, the transaction that records the seek stage's final
result queues a video with no row, and its `face_analysis` entry joins the
import pipeline as `auto_tag` does
([activity.md](activity.md#a-pipeline-groups-one-import-run)).

The queue is filled in three more ways, each refused with
`409 face_recognition_off` while the feature is off.

| Trigger | Queues |
| --- | --- |
| `POST /api/face-recognition/runs` with `missing` | Videos without a row or whose analysis failed |
| `POST /api/face-recognition/runs` with `all` | Every video not running |
| Retry of a `face_analysis` entry in `attention` | Its video, unless it is running; the new entry joins the old one's pipelines |

A recognizer switch, in the settings' transaction, increases the
`face.generation` setting, deletes the old recognizer's face-image vectors and
appearance centroids, and queues again every video whose row is `done` or
`failed`. Appearances keep their tracks, people and crops, so the next analysis
carries them over; a video never analysed stays for "Analyse missing". Claiming
reads the recognizer and the generation in the claim's transaction, so a job
never pairs one recognizer with another's generation. `Store` compares the
job's generation with the current one in its transaction and refuses an older
one with `ErrFaceGenerationChanged`, writing nothing, so a vector of the old
recognizer is never stored beside the new ones. A result for a row that is no
longer `running` writes nothing either.

| Rejected | Why |
| --- | --- |
| Queueing every video in the library on a switch | Turning the feature on with a recognizer other than the default would queue the whole library, while the default queues nothing |
| Keeping the old recognizer's vectors until the videos are analysed again | They cannot be compared with the new ones, and a switch back would mix vectors of two analyses |

## The face worker analyses one video at a time

`FaceRecognizer` ([`face_recognizer.go`](../../internal/app/face_recognizer.go))
claims a video only while the feature is on and the chosen recognizer's
runtime is `ready`; videos queued before that wait, and the runtime wakes the
worker when a download or a helper check makes it ready. The worker resolves
the video's registered location through `internal/mediafs`, as live
transcoding does, and reads its frames with `ffmpeg`
([`face_frames.go`](../../internal/media/face_frames.go)).

| Step | Rule |
| --- | --- |
| Frames | 2 per second; `ffmpeg` applies the display rotation first, then fits the frame into 1280 × 1280 and pads it with black, so a phone video is neither on its side nor stretched |
| Faces | One helper request per frame, at most 2 minutes each, the helper's start included |
| Tracks | The frame's faces go to `FaceTracker` as they arrive; no frame is kept |
| Crops | A crop of each track's best good face is kept while the track can still count: the face box doubled around its centre, clipped to the frame, never enlarged, fitted as a person's face |
| Result | `FindMainPeople` over the tracks, each main person with its best face's crop, stored by `FaceStore.Store` |

The worker keeps one helper per recognizer across videos and stops it when
the queue is empty, the feature is off or the recognizer changes, so the
models leave memory while nothing is analysed.

| Event | Effect |
| --- | --- |
| Turning the feature off or switching the recognizer | The running video is cancelled and goes back to `queued`; the next analysis starts from its first frame |
| A switch that reaches `Store` before the cancellation | `ErrFaceGenerationChanged`; the video goes back to `queued` |
| The helper does not start or greet | The runtime is marked `failed` and the video goes back to `queued`, waiting for the owner to download again |
| The helper exits mid-video, answers with an error or takes over 2 minutes for a frame | The video is `failed` with the message |
| `ffmpeg` fails, or no location can be opened | The video is `failed` with the message |
| The video left the library | The empty result removes its row |
| The server stops | The row stays `running`; startup puts it back to `queued` |

| Rejected | Why |
| --- | --- |
| Letting the running video finish when the feature is turned off | The parent Issue asks for that video to be analysed again when the feature returns |
| Decoding the chosen frames again at the end for the crops | A second pass over the video costs as much as the first |
| Keeping every frame's crops until the end | A long video has thousands of faces; only each track's best one can become a person's face |

## Face data follows its person

Writes to people and credits move or drop face rows in their own transaction
([`face_people.go`](../../internal/store/face_people.go)).

| Write | Face rows |
| --- | --- |
| Deleting a person | Their appearances become dropped and lose centroid and crop, keeping only their tracks; the face image, its vector and the candidate pairs go with the person |
| Merging people | The sources' appearances and candidate pairs move to the target, and a pair that would join the target to itself is deleted; a pair the target already has, in either orientation, keeps the target's row, so each pair of people stays one row; a target that takes a source's face also takes that face's current vector |
| A work-information write that removes a performer | The video's appearances of that person become dropped |
| A work-information write that removes a credit from a merge target | The credit's row of each `face_merge` not yet undone is marked `owner_changed` |

The kept tracks are what let a re-analysis carry a dropped appearance over, so
a deleted face is not recreated and a removed link is not made again.

## An automatic link is undone from the activity log

`FaceStore.UndoFaceLinks` undoes one link of a `face_analysis` entry, or every
link not yet undone, in one transaction
([`face_link_undo.go`](../../internal/store/face_link_undo.go)). A link points
at its appearance, so it follows a merge and a re-analysis that carried the
appearance over; when a re-analysis deleted the appearance, the link's own
person is used.

| State of the link | Undo |
| --- | --- |
| Appearance not dropped, or gone | Drops the appearance, removes the person's `performer` row at the video's current user key unless another appearance of the video not dropped still links the person, and deletes the provisional person the run created when it is still provisional and credited nowhere |
| Appearance dropped | Nothing: the owner removed the performer, undid the link or deleted the person since, and a performer added again afterwards stays |

Each undone link records the owner's `undo` entry in
`activity_entry_face_links.undone_by`; the entry is `already_reverted` when no
link changed anything. Undo removes the performer row even when the owner had
credited that person before the analysis, because a credit's origin is read from
the appearances linked to it, not from the row.

| Rejected | Why |
| --- | --- |
| Detecting the owner's change from activity entries, as auto-tag undo does | Credit edits write no activity entry; the dropped mark that every removal writes is the record |
| Deriving `undone` from the undo entries | An undo entry names no link, and the undo of every link writes one entry for several |

## An automatic merge can be undone

`FaceStore.MergeProvisional` merges a provisional person into another person
as an ordinary merge and records a `face_merge` entry in the same transaction
([`face_merges.go`](../../internal/store/face_merges.go)). The entry's subject
is the target person, and it keeps what the merge moved.

| Record | Contents |
| --- | --- |
| `activity_entry_person_merges` | The source and target ids, and a JSON snapshot: the source's row, profile, favorite, face and vector, appearance ids, candidate pairs and the smart folders whose rule named it; the target's profile before and after, and the favorite and face revision the merge gave it |
| `activity_entry_merge_credits` | Each credit the source had, by user key and role, with `target_had` when the target already had it |

Undo re-creates the source under its old id, which SQLite's `AUTOINCREMENT`
never hands out again, so links to the person page work again. It then moves
back only what the owner has not changed since.

| What the merge moved | Undo |
| --- | --- |
| A credit the target did not have | Moves back to the source in place, unless marked `owner_changed` |
| A credit the target already had | Stays on the target; the source gets its own row after the role's others, unless marked |
| A profile field, the favorite or the face the target took | Taken back only while the target still holds the value the merge gave it |
| Appearances and candidate pairs | Back to the source; the target's copy of a moved pair goes unless the owner decided it since |
| A smart folder rule's person | Back to the source while the rule still names the target |

Editing a video's credits writes no activity entry, so the owner's later
changes cannot be read from entry ids as auto-tag undo reads them. Instead every
work-information write (the details editor, the library selection, plugin
apply and `/api/v1`) marks, in its transaction, the recorded credits it removes
from the target, so a credit the owner removed and added again is the owner's
and stays. A name the owner gives the target is never touched. Undo of an entry
whose target was deleted, or whose source id exists again, records
`already_reverted` and changes nothing.

| Rejected | Why |
| --- | --- |
| Keep merged sources as hidden rows | Every read of people and credits would have to skip them |
| Compare the credits at undo | A credit the owner removed and added again looks unchanged |
| Make the owner's merges undoable too | Only automatic decisions are undone from the activity log, and owner merges write no entry |

## Face entries on the activity screen

Both kinds belong to the `face_recognition` feature of the activity filter. A
`face_merge` entry's subject is the merge target as a `person` subject, whose
item carries `personId` from the subject key, so the card and the detail page
open `/people/:id`; its result names the source as `Unnamed person`, since a
merged source is always provisional. A `face_analysis` entry lists its
`faceLinks` in the list, in the detail and in an import pipeline's member
summaries, so a pipeline's member reads the same names as its own card
(`web/src/activity/activityEntry.ts`).

A card with one link not undone shows `Undo` with that link's `appearanceId`;
with several, `Undo links…` opens a menu of the people and `All links`, which
sends no `appearanceId`. The detail page's `People` section lists every link
with its own undo, and strikes through an undone one. An undo of one named
link keeps the person's name as its `detail`, read as `Undid the link to
{name}`; the undo of an unnamed person's link or of every link reads
`Undid Face analysis`.

## The owner settles a candidate pair

A pair in `face_candidates` is stored once as `(provisional_id, person_id)`,
and the people API names it by its two people in either order
([`face_candidates.go`](../../internal/store/face_candidates.go)).
`GET /api/people/{id}/face-candidates` lists the open pairs a person takes part
in as the other person of each, highest similarity first.

| Action | Effect |
| --- | --- |
| Confirm | The ordinary merge of the pair's person without a canonical name into the other, in one transaction with the pair lookup; it records no `face_merge` entry, since the owner made it |
| Dismiss | Sets `dismissed`; a later result that offers the same pair updates only its similarity, so the pair stays hidden |

When both people have a name, or neither has, confirm merges the stored
provisional side: a name given after the pair was offered does not reverse
it. Without an open pair, both actions answer `404 face_candidate_not_found`.

## The person page reads a person's appearances

`GET /api/people/{id}/appearances` lists a person's appearances that are not
dropped, with the admin lists' `limit` and `cursor`
([`face_appearance_list.go`](../../internal/store/face_appearance_list.go)).
They come newest video first by the video's added time, then by descending
appearance id. An appearance whose content key no video has any more is still
listed, without `videoId` and after every appearance with a video, so the
owner sees that the person was found in a file that has left the library.
Each item's spans are its tracks' times in order, with overlapping tracks
joined.

`GET /api/face-appearances/{id}/crop` answers the crop with
`Cache-Control: private, max-age=86400`: a crop is never rewritten, and a
re-analysis gives a new appearance a new id. A missing appearance and one whose
person was deleted, which has no crop, both answer `404 not_found`.

| Rejected | Why |
| --- | --- |
| Hiding appearances of content no video has | A returning file keeps its links, and the person page would show nothing for a person found only there |
| Ordering by appearance id alone | A re-analysis keeps old ids, so the order would follow the first analysis, not the library |
