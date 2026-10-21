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

| Rejected              | Why                                                                     |
| --------------------- | ----------------------------------------------------------------------- |
| `purego` inside `mdm` | `mdm` no longer starts on Alpine                                        |
| A glibc base image    | Every user's image changes for an optional feature                      |
| Inference in pure Go  | The convolutions would have to be written and kept equal to onnxruntime |

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

| Message      | Rule                                                                                                                                      |
| ------------ | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `Hello`      | The helper's first message; a different `Version` is an error on read, so a stale archive is reported instead of misread                  |
| `Error`      | Replaces `Hello` when the models do not load, and answers a request that failed; the helper keeps serving after a failed request          |
| `Detections` | Every face YuNet keeps (score 0.6, non-maximum suppression at IoU 0.3), each with its vector; tracking and the quality gate stay in `mdm` |

Each message is a little-endian `uint32` length, a kind byte and its payload,
at most 64 MiB. The helper exits when its input closes.

## Two recognizers share one detector

YuNet 2023mar detects faces for every recognizer; the recognizer is SFace
2021dec int8 (`sface_int8`) or ArcFace r50 from InsightFace `buffalo_l`
(`arcface_r50`).

| Recognizer    | Input                           | Vector |
| ------------- | ------------------------------- | ------ |
| `sface_int8`  | Planar RGB, 0..255              | 128    |
| `arcface_r50` | Planar RGB, (v − 127.5) / 127.5 | 512    |

Both take the same five-point alignment to a 112 × 112 template, so a switch
never changes which faces are found. The recognizer's input and output names
are read from the model, because exports of one model name them differently.
The frame path aligns without rounding to bytes, as the measured proof of
concept did: on LFW the Go SFace int8 pipeline reached 99.82% accuracy against
OpenCV's 99.86% with SFace fp32.

SFace is the default because its license allows commercial use; ArcFace's
model is licensed for non-commercial research only, which the settings page
shows on the option.

| Rejected   | Why                                                             |
| ---------- | --------------------------------------------------------------- |
| SFace fp32 | 0.04 points of LFW accuracy for four times the download (37 MB) |

## The face runtime archive

CI builds one archive per platform from
[`scripts/build/face_runtime.go`](../../scripts/build/face_runtime.go)
(`go run ./scripts/build -face-runtime <platform>`), and the
[face-runtime workflow](../../.github/workflows/face-runtime.yml) publishes
them as a `face-runtime-v<N>` GitHub Release when the maintainer pushes that
tag.

| Archive                                                              | Contents                                                                                                                                                                                            |
| -------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `face-runtime-linux-amd64.tar.gz`, `face-runtime-linux-arm64.tar.gz` | `mdm-face`; `lib/` with Microsoft's onnxruntime 1.22.0 and the loader, `libc`, `libm`, `libdl`, `libpthread`, `librt`, `libstdc++` and `libgcc_s` of a pinned `debian:trixie` image; their licenses |
| `face-runtime-windows-amd64.zip`                                     | `mdm-face.exe`, `onnxruntime.dll`, its license                                                                                                                                                      |

On Linux the helper starts as `lib/ld-linux-*.so --library-path lib mdm-face`,
so it runs under the bundled glibc on any host, Alpine included. Each pull
request that changes the helper builds the three archives, and the Linux jobs
start each helper in `alpine:3.24` through that loader on one photo and expect
one face with a 128-value vector. A separate tag keeps nightly and tagged vv
builds on the same archive, which changes only with onnxruntime or the
protocol.

| Rejected                                         | Why                                                                                                          |
| ------------------------------------------------ | ------------------------------------------------------------------------------------------------------------ |
| The runtime in the image and the Windows zip     | About 26 MB for every user, including those who never turn the feature on                                    |
| onnxruntime and Debian packages fetched by `mdm` | `mdm-face` still has to come from vv, and unpacking `.deb` files adds a format the server needs nowhere else |
| A runtime per vv release                         | Nightly and development builds would have nothing to download                                                |

## The runtime is downloaded only when the feature is on

`mdm` pins the URL and SHA-256 of each platform's runtime archive and of each
model in code
([`internal/faceruntime`](../../internal/faceruntime/catalog.go)), and
downloads them only when the owner turns face recognition on, or asks to
download again after a failure. The models come from their upstream Hugging
Face repositories, each URL pinned to a commit.

| File               | Source                                                                         | Size           |
| ------------------ | ------------------------------------------------------------------------------ | -------------- |
| Runtime archive    | The `face-runtime-v1` release (`linux-amd64`, `linux-arm64`, `windows-amd64`)  | 5.9 to 11.6 MB |
| YuNet 2023mar      | `opencv/face_detection_yunet` (MIT)                                            | 0.2 MB         |
| SFace 2021dec int8 | `opencv/face_recognition_sface` (Apache-2.0)                                   | 9.9 MB         |
| ArcFace r50        | `immich-app/buffalo_l`, `recognition/model.onnx` (InsightFace, non-commercial) | 174 MB         |

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

| Event                                                                        | Effect                                                                                                                                                                 |
| ---------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Turning the feature on, or switching the recognizer while on                 | Downloads what the recognizer needs that is missing or differs; a download for the other recognizer is cancelled                                                       |
| Turning the feature off                                                      | Cancels a running download; the files already verified stay, and a recognizer whose helper had not greeted is `absent` until the next start probes it                  |
| Startup while on                                                             | Resumes a download only when files are missing; a file that differs stays `failed` until the owner downloads again                                                     |
| A download ends                                                              | The helper is started once: a `Hello` of another protocol version, or no `Hello`, is `helper_start`                                                                    |
| Any helper start                                                             | The runtime and the recognizer's models are hashed against their pins again first; a helper whose files no longer match is not started and is `hash_mismatch`          |
| Any download start, downloading again included                               | The files the recognizer needs are hashed again rather than taken from an earlier check, so a file deleted or changed since then is downloaded again                   |
| Any start of a recognizer whose helper has not greeted, with no file missing | The helper is started once again instead of the state turning `ready` unchecked: after a failure, a cancelled handshake, or switching back to a recognizer that failed |
| A recognizer switch                                                          | Increases `face.generation` in the settings' transaction                                                                                                               |

On Linux `mdm` starts `runtime-v<N>/lib/<loader> --library-path
runtime-v<N>/lib runtime-v<N>/mdm-face`; on Windows it starts `mdm-face.exe`
with `CREATE_NO_WINDOW`, through the same command builder as `ffmpeg`. The
helper gets half of the CPU cores, at least one, for onnxruntime's threads, so
playback and live transcoding keep the rest. The client starts the helper on
its first request; a helper whose stream breaks fails that request and is
started again on the next one, and a cancelled request stops it.

| Rejected                                                                              | Why                                                                                                                                                      |
| ------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Storing the download state in the settings                                            | It would disagree with the files after a crash or a deleted folder; the hashes are the state                                                             |
| Keeping the runtime archive to verify it at startup                                   | It doubles the disk use; the per-file pins built into `mdm` verify the unpacked files                                                                    |
| A hash manifest written into the runtime folder                                       | Anyone who can write the folder could replace `mdm-face` and its hash together; the trust root must be the `mdm` binary                                  |
| Starting the helper from a file descriptor or an immutable copy of the verified files | A short window between the check and the start remains, but a writer of the data folder as the service user can already change the database and settings |
| Downloading again at startup a file that differs                                      | The owner would not see that a file broke; the settings page shows `failed` and offers the download                                                      |

## From faces to people

`mdm` turns the helper's faces into people with the rules below, one video at
a time. The diagram shows how one video's faces become links, candidates and merges.

```mermaid
flowchart LR
  frames[Faces of 1 fps frames] --> tracker[FaceTracker: tracks]
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

A video's main people are clusters of its face tracks with enough seconds of
good faces for the video's analysed length
([`face_main_people.go`](../../internal/domain/face_main_people.go)).

The constants were measured together in the face POC on 12 real videos, where
20 of 22 main people held one person only; they are code, not settings, because
the quality gate and the cutoff only work as a pair. The POC videos were long,
so the fixed 8 seconds in 2 tracks came from them; the length-aware bar below
is a later extension that keeps 8 seconds for long videos. The sampling rate
is a later measurement: on real 15-minute videos at 1080p and 480p, with the
real runtime, SFace int8 and 2 helper threads on 4 cores, 1 frame per second
found the same main people as 2 (cosine similarity 0.93 or more between
matched people, no extra or merged people) in 1.6 to 2.6 times less time.
The frame side made no difference to speed, because the helper runs YuNet at
640 px internally, so 1280 px stays for crop quality.

| Step                    | Rule                                                                                                                                                                                                                                                                                                                                      |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Sampling                | 1 frame per second, fitted into 1280 × 1280                                                                                                                                                                                                                                                                                               |
| Tracking                | Faces under 48 px are skipped; a face continues the open track it is most similar to above `Track` when the boxes overlap (IoU over 0.05), or above `TrackFar` when they do not, and the most similar face and track pairs of a frame are joined first; a track closes after 2 missed frames (2 s) and is kept with 2 or more faces (2 s) |
| Quality gate            | A good face is 64 px wide or more, scores 0.85 or more, and has its nose within 0.5 ± 0.22 of the way between the eyes                                                                                                                                                                                                                    |
| Track vector            | The normalized mean of the good faces; a track with fewer than 2 good faces has none and is not clustered                                                                                                                                                                                                                                 |
| Clustering              | Average linkage over the track vectors, joining while the mean similarity reaches `Cluster`                                                                                                                                                                                                                                               |
| Cutoff                  | Good seconds (one per good face) at least the bar, the larger of 3 s and 25% of the analysed length. A cluster of 2 or more tracks needs only min(8 s, bar); a cluster of one track needs the bar itself                                                                                                                                  |
| Merge of split clusters | Clusters that passed the cutoff and whose centroids reach `Join` are one person and are joined, repeatedly, most similar first                                                                                                                                                                                                            |

The analysed length is the number of frames the worker sampled divided by the
sampling rate, not the file's duration, so a video cut short is judged by what
was seen. The 3 s floor keeps a cameo from counting in a short clip; 25% means
a person on screen for a quarter of the video is a main person, which is what
a 15 s clip with one centred face needs (3.75 s), while 8 s still decides in any
video of 32 s or more. A single track is the only evidence of one continuous
shot, so in a long video it must reach the full 25%: 10 s of one track in a
100 s video is not a main person, 25 s is.

Average linkage at `Cluster` can leave one person's tracks in two clusters, for
example before and after a change of lighting. A cluster centroid averages many
faces, so it reaches `Join` where the average linkage of single faces stays under
`Cluster`; the merge joins such clusters automatically. `Candidate` is too loose
to join people without the owner (it names a stranger for over half of unknown
faces in the POC), so two provisional people of one analysis whose centroids
reach only `Candidate` are written as an open `face_candidates` pair, and the person's
faces page (`/people/:id/faces`) offers "Same person?".

Each track keeps its start, end, mean centre and mean width, and a main person
keeps its best good face for its crop and its face vector: a face whose box lies
inside the picture before any face cut by the picture's edge, then the largest
box times score times frontality, which is 1 for a face turned to the camera and
falls to 0 as the nose reaches an eye. The
picture is the part of the frame that any frame of the video so far lit, so the
black padding and black bars stay outside it while a dark scene does not shrink
it once a brighter frame was seen. A
close-up cut by the edge has the largest box, so ranking by size alone made such
faces the faces of people.

| Rejected                              | Why                                                                    |
| ------------------------------------- | ---------------------------------------------------------------------- |
| Clustering the whole library together | Every import would re-cluster every video, and person ids could change |

## Each recognizer has its own thresholds

Similarity thresholds are constants in the recognizer table
([`face_recognizer.go`](../../internal/domain/face_recognizer.go)), because
SFace and ArcFace vectors spread differently and the owner has no data to set
them.

| Threshold   | Use                                        | SFace int8 | ArcFace r50 |
| ----------- | ------------------------------------------ | ---------- | ----------- |
| `Track`     | Continue a track, boxes overlapping        | 0.45       | 0.288       |
| `TrackFar`  | Continue a track, boxes apart              | 0.6        | 0.614       |
| `Cluster`   | Join clusters of tracks                    | 0.45       | 0.288       |
| `Join`      | Join split clusters of one video           | 0.45       | 0.288       |
| `Candidate` | Offer a pair to the owner                  | 0.363      | 0.205       |
| `Link`      | Link a main person to a person             | 0.55       | 0.545       |
| `Merge`     | Merge a provisional person on a face image | 0.55       | 0.545       |

The SFace values are the upstream same-person threshold and the POC's. The
ArcFace values give the same error rates as SFace on LFW (13,233 images of
5,749 people), measured with the POC's protocol on vectors from the same
detection and alignment:

- **Late linking** (`Candidate`, `Join`, `Link`, `Merge`): 70% of the people with two
  or more images are registered with one random image each; every other image
  is named by its most similar registered person when the similarity reaches
  the threshold. A correct naming is a registered person's image given their
  own name; a false naming is an unregistered person's image given any name.
  Rates are the mean of 5 random draws.
- **Pairs** (`Track`, `TrackFar`, `Cluster`): the share of all pairs of images
  of two different people at or above the threshold.

The measured rates, with the 4,950 registered and 7,107 unregistered images
of a draw and the 87,307,271 pairs of different people:

| Threshold                | SFace int8: value, rate           | ArcFace r50: value, rate          |
| ------------------------ | --------------------------------- | --------------------------------- |
| `Candidate`              | 0.363: 98.3% correct, 61.6% false | 0.205: 99.8% correct, 61.3% false |
| `Join`, the first `Link` | 0.45: 96.0% correct, 3.43% false  | 0.288: 99.7% correct, 3.42% false |
| `Link`, `Merge`          | 0.55: 83.9% correct, 0.17% false  | 0.545: 90.4% correct, 0.17% false |
| `Track`, `Cluster`       | 0.45: 4,482 pairs (0.0051%)       | 0.288: 4,442 pairs                |
| `TrackFar`               | 0.6: 88 pairs (0.0001%)           | 0.614: 87 pairs                   |

At equal false rates ArcFace names more people correctly, most of all at
`Merge`. An unregistered image named at `Candidate` is why that threshold only
offers a pair and never links.

`Link` takes the value of `Merge`, the 0.17% false rate, because a link is made
without the owner as a merge is. At the first values (0.45 and 0.288, the 3.4%
rate), the maintainer's library had most new people linked to strangers; video
faces are smaller and blurrier than LFW's. A face between `Candidate` and `Link`
becomes a provisional person offered with the near people, which the owner
confirms with "Same person?". `Join` keeps the first value, because it compares
the clusters of one video, where few people compete.

| Rejected                      | Why                                                                                                           |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `Link` at the 3.4% false rate | Most new people were linked to strangers in the maintainer's library, and every wrong link widened its person |

## A person is matched by their face alone

Each person has one vector, that of their face image, and a main person's
centroid is compared with it
([`MatchFace`](../../internal/domain/face_match.go),
[`facePersonVectors`](../../internal/store/faces.go)).

| Similarity               | Result                                                                                                                |
| ------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| `Link` or more           | Linked to the most similar person, and each other person at `Link` or more is offered with that person as a candidate |
| `Candidate` up to `Link` | A new provisional person, offered with each such person as a candidate                                                |
| Below `Candidate`        | A new provisional person                                                                                              |

A person made by an analysis gets the crop of their best face as their face
image and the vector of that face, taken from its frame, as its vector, in the
transaction that creates them; the face worker finds the vector current and
does not embed the crop again. The face changes only when the owner replaces
it, or a merge gives a target without a face the source's, and the vector
follows the face. Links never change it.

Two people at `Link` or more are nearly always one person stored twice, as when
an earlier video of them stayed under `Link` and made a provisional person:
strangers reach `Link` for 0.17% of faces.

| Rejected                                                             | Why                                                                                                                                               |
| -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| Linking only when the most similar person leads the next by a margin | One person stored twice never leads, so every video of them made one more provisional person                                                      |
| Every appearance's centroid as a vector of its person                | A person seen in many videos had as many chances to match a stranger, and each wrong link widened the person, so a few people took every new face |
| One averaged vector per person                                       | Each link would move it, and a wrong link would pull it towards the stranger                                                                      |
| Embedding the crop of a new person in the face worker                | The JPEG crop gives a vector other than its frame's, and the next video could not match the person until the worker ran                           |
| A nearest-neighbour index                                            | 10,000 vectors of 512 values are 20 MB, and a full scan in memory takes milliseconds                                                              |

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

| Rejected                                      | Why                                                                                                              |
| --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Time overlap alone                            | Two people on screen together overlap each other's spans, so their people could swap                             |
| Matching the carried-over centroid again      | After a switch the other videos' vectors still belong to the old recognizer, and links the owner kept would move |
| A fixed fraction of the frame as the distance | A close-up and a wide shot need different distances; the face width scales with the shot                         |

## A face image merges only one provisional person

When a named person's face image reaches `Merge` with exactly one provisional
person, that person is merged into them; two or more at `Merge`, and every
provisional person from `Candidate` up, are candidates
([`DecideFaceImageMerge`](../../internal/domain/face_match.go)). A provisional
person's own face merges nobody, so provisional people never merge into each
other on their own. A provisional person whose pair with the named person the
owner dismissed is neither merged nor counted.

The face worker embeds faces between videos
([`face_images.go`](../../internal/app/face_images.go)). A face's vector is the
vector of its only good face, by the quality gate of video faces; a face with
none or several gets a null vector. `PersonImageChanged`, published after the
commit that sets, replaces or brings a face by a merge, or names a provisional
person with a face, wakes the worker for that person; the worker also takes any
face whose `person_face_vectors` row is missing or of another revision or
recognizer, so a lost event or a recognizer switch still leads to a vector.

| Step  | Rule                                                                                                                                                                                                                                                                                                                                                                                                                             |
| ----- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Read  | The face, its stored vector, the recognizer and `face.generation`, in one read                                                                                                                                                                                                                                                                                                                                                   |
| Embed | Skipped when the stored vector is current, as after a naming; one helper request, at most 2 minutes, on the video's helper                                                                                                                                                                                                                                                                                                       |
| Store | `FaceStore.StoreFaceImageVector`, in one transaction: refuses an older generation with `ErrFaceGenerationChanged`, writes nothing for a face changed since, writes the vector, then applies the rule above and merges as a `face_merge` entry ([An automatic merge can be undone](#an-automatic-merge-can-be-undone)); a merge refused for good is rolled back alone ([below](#an-automatic-merge-refused-for-good-is-left-out)) |

| Event                                                                                  | Effect                                                                                             |
| -------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| The settings change during the embedding, or the vector reaches `Store` after a switch | The face is read again under the new settings                                                      |
| The helper does not start                                                              | The face waits until the runtime is ready again                                                    |
| The helper exits, answers with an error or takes over 2 minutes                        | A null vector, as for a face without a good face; the face is not embedded again until it changes  |
| A database error                                                                       | The worker tries again after 5 seconds                                                             |
| The merge is refused for good                                                          | The vector and candidates are stored without the merge, and the worker logs the person and goes on |

While the feature is on and its runtime is `ready`, `PUT /api/people/{id}/image`
detects the faces of the fitted image before storing it, and refuses an image
without a good face with `400 person_image_no_face` and one with several with
`400 person_image_many_faces`. The check starts its own helper, one check at a
time, so a running video goes on; when the helper cannot answer, the face is
stored unchecked. Plugin faces are not checked.

| Rejected                                          | Why                                                                                                                                              |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| Embedding inside the upload request               | Plugin and merge faces are stored after their own commits, and the worker's helper may be in a video                                             |
| Embedding only the faces named by events          | An event is lost when the server stops before the worker runs, and a recognizer switch publishes none; the stale rows are read from the database |
| Sharing the worker's helper with the upload check | The worker stops its helper when idle, and a request from another goroutine would start a process nobody stops                                   |
| Checking plugin faces on upload too               | Only the owner's upload is checked; a plugin face without one good face gets a null vector and merges nobody                                     |

## The cast rule merges only one-to-one pairs

When exactly one performer of a video has no face linked in it and exactly one
provisional person is linked to it by face, the provisional person is merged
into that performer, unless the performer is provisional too or the owner
dismissed the pair
([`DecideCastMerge`](../../internal/domain/face_match.go)).

The performer has no face to compare with, so the rule relies on the
one-to-one pairing; in the POC, five candidates from a cast list named 0.5% of
unknown faces wrongly at 0.363, and one candidate is stricter still.

The rule reads a work's performers, their appearances in every video sharing
the work, and the dismissed pairs in the transaction that merges
([`face_cast.go`](../../internal/store/face_cast.go)), and records the merge as
a `face_merge` entry.

| Trigger                                           | Where the rule runs                                                                                                                             |
| ------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| A video's analysis ends                           | In `FaceStore.Store`'s transaction, after the result is written                                                                                 |
| A work-information write (`VideoWorkInfoChanged`) | In the face worker, before its next claim, while the feature is on; a database error keeps the unchecked videos for the attempt after 5 seconds |

A merge refused for good is left out as the
[next section](#an-automatic-merge-refused-for-good-is-left-out) describes:
`Store` keeps the result without the merge, and the worker logs the video and
drops its check.

| Rejected                                                                            | Why                                                                                                                                             |
| ----------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Running the rule in every work-information write's transaction                      | The details editor, the library selection, plugin apply and `/api/v1` would each record activity and fail the owner's edit when the merge fails |
| Checking work-information changes made while the feature is off once it is on again | Nothing records them, and the next analysis of the video applies the rule anyway                                                                |

## An automatic merge refused for good is left out

Both automatic rules merge through one helper,
`mergeProvisionalOrRefuse` ([`face_cast.go`](../../internal/store/face_cast.go)),
which runs the ordinary merge under a savepoint. A refusal that trying again
cannot change (the target's limits on aliases or external IDs, a source that is
no longer provisional, or the two being one credit) is rolled back to that
savepoint and wraps `ErrFaceMergeRefused`, so the rest of the transaction is
kept.

| Rule                                      | What is kept                         | Worker                                  |
| ----------------------------------------- | ------------------------------------ | --------------------------------------- |
| Cast rule at the end of an analysis       | The analysis result                  | —                                       |
| Cast rule after a work-information change | Nothing to keep                      | Logs the video and drops its check      |
| Face-image rule                           | The face's vector and its candidates | Logs the person and takes the next face |

The face's vector is stored even without the merge, because a face without a
current vector is read again as stale and would be embedded and refused again
every 5 seconds.

| Rejected                                     | Why                                                                                        |
| -------------------------------------------- | ------------------------------------------------------------------------------------------ |
| Trying a refused merge again after 5 seconds | It cannot succeed until the owner changes the people, and every attempt holds up the queue |
| Offering a refused pair as a candidate       | Confirming it is the ordinary merge, which the same limits refuse                          |

## One video's result is stored in one transaction

`FaceStore.Store` writes a video's analysis in one transaction
([`faces.go`](../../internal/store/faces.go)): it re-reads the stored
appearances and every person's vectors, applies `PlanFaceAppearances`, and
writes what the plan decides. Reading the people inside the transaction means a
person deleted during the analysis is never linked.

| Plan for a main person | What is written                                                                                                                                                      |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Carried over           | The row keeps its id, person, dropped mark and crop, and takes the new tracks, counts, recognizer and centroid; a row whose person was deleted keeps a null centroid |
| Linked to a person     | A new appearance and a `performer` row at the video's user key                                                                                                       |
| New                    | A provisional person (a `people` row without names) whose face is the crop with its face's vector, its candidate pairs, a new appearance and a `performer` row       |

A stored appearance nothing is carried over to is deleted. The completed
`face_analysis` entry lists each link it made in
`activity_entry_face_links`, with `created_person` set when the run created the
person. Appearances are keyed by content key, so a content key that no video
has any more keeps its rows, and a returning file keeps its links.

| Rejected                                   | Why                                                                                                                 |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------- |
| Matching in the worker, then writing       | A person deleted or merged between the read and the write would be linked                                           |
| Keeping crops only on appearances          | The people grid, the cast chips and the candidates show `person_images`, so a provisional person would show no face |
| A 112 × 112 aligned crop as the face       | It is the model's input, cut tight to the eyes and mouth: too small and too tight for a portrait                    |
| A face-link table beside credits           | The person page, the filters and smart folders read credits only; every reader would have to change                 |
| A `source` column on `video_work_people`   | The details editor and the plugin apply replace the rows, so every edit would lose it                               |
| A table of rejected person and video pairs | A deleted person leaves no id to reject, and a pair would not follow a re-analysis as tracks do                     |

## The face queue mirrors the auto-tagging queue

`face_queue` holds one row per content key in the states `queued`, `running`,
`done` and `failed`, and shares its queueing, claiming, failure and recovery
code with `auto_tag_queue`
([`content_queue.go`](../../internal/store/content_queue.go)). While
`face.enabled` is `true`, the transaction that records the seek stage's final
result queues a video with no row, and its `face_analysis` entry joins the
import pipeline as `auto_tag` does
([activity.md](activity.md#a-pipeline-groups-one-import-run)). A forced seek
job, queued by a force rescan, queues its video whether it has no row or is
`done` or `failed`, and leaves a `queued` or `running` row, with an owner's entry that joins the same pipeline
([force rescan](force-rescan.md#tagging-and-face-analysis-run-again)).

The queue is filled in two more ways, each refused with
`409 face_recognition_off` while the feature is off.

| Trigger                                          | Queues                                                                       |
| ------------------------------------------------ | ---------------------------------------------------------------------------- |
| `POST /api/face-recognition/runs` with `missing` | Videos without a row or whose analysis failed                                |
| Retry of a `face_analysis` entry in `attention`  | Its video, unless it is running; the new entry joins the old one's pipelines |

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

| Rejected                                                                            | Why                                                                                                                             |
| ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| A run that queues every analysed video again (`all`)                                | A re-analysis carries each appearance over to the same person, so it spent hours to find the same people                        |
| A job kind in `jobs`                                                                | Import progress counts jobs, and an optional analysis at about 0.27 times the video length would hold the import open           |
| Queueing every video in the library on a switch                                     | Turning the feature on with a recognizer other than the default would queue the whole library, while the default queues nothing |
| Keeping the old recognizer's vectors until the videos are analysed again            | They cannot be compared with the new ones, and a switch back would mix vectors of two analyses                                  |
| Cancelling the worker and waiting for it before a switch, instead of the generation | An embedding an event queues after the wait would still write the old recognizer's vector                                       |
| A lock held for a whole analysis                                                    | An analysis takes minutes, and the settings request would wait for it                                                           |

## The settings page

**Face recognition** in Settings
([`FaceRecognitionSection.tsx`](../../web/src/settings/FaceRecognitionSection.tsx))
shares the Auto-tagging page's `Progress` section, with only its "Analyse videos
not yet analysed" button. It reads
`GET /api/settings/face-recognition` on opening, every second while the
runtime is `downloading` and every three seconds while videos are queued or
running. The download's progress and failure sit under the switch that
started it, with `Download again` on a failure.

| Decision                                                                                         | Why                                                                                             |
| ------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------- |
| A recognizer change while on is confirmed first, and the radio moves only when the `PUT` answers | One press queues every video again; a failed save must leave the saved recognizer shown         |
| The non-commercial license is shown on the option itself                                         | The owner must see it before choosing, not after                                                |
| `unsupported` is a page notice that keeps every control visible with `aria-disabled`             | It is the one condition that locks the whole page, as the Ollama lock does on Auto-tagging      |
| The recognizer is a radio group, not a select                                                    | Two options read once, and a select would hide the license until opened                         |
| Progress is polled from the settings resource, not sent as an SSE event                          | One screen watches a download that happens once, and Auto-tagging polls its counts the same way |

## The face worker analyses one video at a time

`FaceRecognizer` ([`face_recognizer.go`](../../internal/app/face_recognizer.go))
claims a video only while the feature is on and the chosen recognizer's
runtime is `ready`; videos queued before that wait, and the runtime wakes the
worker when a download or a helper check makes it ready. The worker resolves
the video's registered location through `internal/mediafs`, as live
transcoding does, and reads its frames with `ffmpeg`
([`face_frames.go`](../../internal/media/face_frames.go)).

| Step   | Rule                                                                                                                                                                                                                                                                       |
| ------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Frames | 2 per second; `ffmpeg` applies the display rotation first, then fits the frame into 1280 × 1280 and pads it with black, so a phone video is neither on its side nor stretched                                                                                              |
| Faces  | One helper request per frame, at most 2 minutes each, the helper's start included                                                                                                                                                                                          |
| Tracks | The frame's faces go to `FaceTracker` as they arrive; no frame is kept                                                                                                                                                                                                     |
| Crops  | A crop of each track's best good face is kept while the track can still count: the face box doubled around its centre, moved inside the picture where it reaches past its edge and cut only where it is larger than the picture, never enlarged, fitted as a person's face |
| Result | `FindMainPeople` over the tracks, each main person with its best face's crop, stored by `FaceStore.Store`                                                                                                                                                                  |

The worker keeps one helper per recognizer across videos and stops it when
the queue is empty, the feature is off or the recognizer changes, so the
models leave memory while nothing is analysed.

| Event                                                                                                                | Effect                                                                                                                                      |
| -------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| Turning the feature off or switching the recognizer                                                                  | The running video is cancelled and goes back to `queued`; the next analysis starts from its first frame                                     |
| A switch that reaches `Store` before the cancellation                                                                | `ErrFaceGenerationChanged`; the video goes back to `queued`                                                                                 |
| The helper does not start or greet                                                                                   | The runtime is marked `failed` and the video goes back to `queued`, waiting for the owner to download again                                 |
| The helper exits mid-video, answers with an error or takes over 2 minutes for a frame                                | The video is `failed` with the message                                                                                                      |
| `ffmpeg` fails, or no location can be opened                                                                         | The video is `failed` with the message                                                                                                      |
| The video left the library                                                                                           | The empty result removes its row                                                                                                            |
| A database error while reading the video's locations or the media folders, storing the result or recording a failure | The video goes back to `queued` and the worker tries again after 5 seconds; if that also fails, the worker returns it before its next claim |
| The server stops                                                                                                     | The row stays `running`; startup puts it back to `queued`                                                                                   |

| Rejected                                                        | Why                                                                                        |
| --------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| Letting the running video finish when the feature is turned off | The parent Issue asks for that video to be analysed again when the feature returns         |
| Decoding the chosen frames again at the end for the crops       | A second pass over the video costs as much as the first                                    |
| Keeping every frame's crops until the end                       | A long video has thousands of faces; only each track's best one can become a person's face |

## Face data follows its person

Writes to people and credits move or drop face rows in their own transaction
([`face_people.go`](../../internal/store/face_people.go)).

| Write                                                              | Face rows                                                                                                                                                                                                                                                                                                                  |
| ------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Deleting a person                                                  | Their appearances become dropped and lose centroid and crop, keeping only their tracks; the face image, its vector and the candidate pairs go with the person                                                                                                                                                              |
| Merging people                                                     | The sources' appearances and candidate pairs move to the target, and a pair that would join the target to itself is deleted; a pair the target already has, in either orientation, keeps the target's row, so each pair of people stays one row; a target that takes a source's face also takes that face's current vector |
| A work-information write that removes a performer                  | The video's appearances of that person become dropped                                                                                                                                                                                                                                                                      |
| A work-information write that removes a credit from a merge target | The credit's row of each `face_merge` not yet undone is marked `owner_changed`                                                                                                                                                                                                                                             |

The kept tracks are what let a re-analysis carry a dropped appearance over, so
a deleted face is not recreated and a removed link is not made again.

| Rejected                                    | Why                                                                    |
| ------------------------------------------- | ---------------------------------------------------------------------- |
| Deleting a deleted person's appearances too | The next analysis would find the same face and create the person again |

## Deleting every person starts the people over

`DELETE /api/people` deletes every person, named or provisional, and every
face appearance in one transaction
([`FaceStore.ClearPeople`](../../internal/store/face_clear.go)), and the face
analysis then starts from nothing. **Delete all people…** on the Settings
**Danger zone** page
([`ClearPeopleSection.tsx`](../../web/src/settings/ClearPeopleSection.tsx))
asks first; the page holds the operations that cannot be undone, so none of
them sits beside an everyday setting.

| Step              | Effect                                                                                                                                                       |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `face.generation` | Increases, so an analysis or face embedding that started before writes nothing and its video is analysed again                                               |
| Face appearances  | Deleted, not marked dropped as a single person's delete marks them; a dropped row would be carried over by the next analysis and its face never linked again |
| People            | Deleted with their names, external IDs, credits on videos, profiles, favorites, faces, face vectors and candidate pairs                                      |
| `face_queue`      | `done` and `failed` rows go, so every video counts as not analysed; while the feature is on every video is queued, with an owner's `face_analysis` entry     |

Smart folders whose rule names a person then match nothing, as after any
person's delete. Activity entries stay: a link or merge whose person is gone
undoes nothing, as after any delete. The operation works with the feature off
and without a runtime.

| Rejected                                               | Why                                                                                                                         |
| ------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------- |
| Deleting people one by one through the ordinary delete | It marks each appearance dropped, so no later analysis would make those people again                                        |
| A section of the Face recognition page                 | It deletes every person and their credits, far beyond face recognition, beside the switches the owner changes every day     |
| Deleting face data only and keeping the people         | The people's faces and credits would no longer match any appearance, and the next analysis would make a second copy of each |

## An automatic link is undone from the activity log

`FaceStore.UndoFaceLinks` undoes one link of a `face_analysis` entry, or every
link not yet undone, in one transaction
([`face_link_undo.go`](../../internal/store/face_link_undo.go)). A link points
at its appearance, so it follows a merge and a re-analysis that carried the
appearance over; when a re-analysis deleted the appearance, the link's own
person is used.

| State of the link               | Undo                                                                                                                                                                                                                                                                       |
| ------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Appearance not dropped, or gone | Drops the appearance, removes the person's `performer` row at the video's current user key unless another appearance of the video not dropped still links the person, and deletes the provisional person the run created when it is still provisional and credited nowhere |
| Appearance dropped              | Nothing: the owner removed the performer, undid the link or deleted the person since, and a performer added again afterwards stays                                                                                                                                         |

Each undone link records the owner's `undo` entry in
`activity_entry_face_links.undone_by`; the entry is `already_reverted` when no
link changed anything. Undo removes the performer row even when the owner had
credited that person before the analysis, because a credit's origin is read from
the appearances linked to it, not from the row. The exception is a link whose
person a `face_merge` not undone moved into the person it now points to: when
that merge recorded the credit with `target_had`, the target had it before the
merge, so the link did not add it and undo keeps the row.

| Rejected                                                                  | Why                                                                                            |
| ------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Detecting the owner's change from activity entries, as auto-tag undo does | Credit edits write no activity entry; the dropped mark that every removal writes is the record |
| Deriving `undone` from the undo entries                                   | An undo entry names no link, and the undo of every link writes one entry for several           |

## An automatic merge can be undone

`FaceStore.MergeProvisional` merges a provisional person into another person
as an ordinary merge and records a `face_merge` entry in the same transaction
([`face_merges.go`](../../internal/store/face_merges.go)). The entry's subject
is the target person, and it keeps what the merge moved.

| Record                         | Contents                                                                                                                                                                                                                                                                     |
| ------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `activity_entry_person_merges` | The source and target ids, and a JSON snapshot: the source's row, profile, favorite, face and vector, appearance ids, candidate pairs and the smart folders whose rule named it; the target's profile before and after, and the favorite and face revision the merge gave it |
| `activity_entry_merge_credits` | Each credit the source had, by user key and role, with `target_had` when the target already had it                                                                                                                                                                           |

The faces page's `Undo` of a confirmed pair uses the same entry, so
`UndoOffered` accepts a `face_merge` decided by the owner as it does an
automatic one; the page then reopens the pair, which undo marks dismissed.

Undo re-creates the source under its old id, which SQLite's `AUTOINCREMENT`
never hands out again, so links to the person page work again. It then moves
back only what the owner has not changed since.

| What the merge moved                                      | Undo                                                                                                                                             |
| --------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| A credit the target did not have                          | Moves back to the source in place, unless marked `owner_changed`                                                                                 |
| A credit the target already had                           | Stays on the target; the source gets its own row after the role's others, unless marked                                                          |
| A profile field, the favorite or the face the target took | Taken back only while the target still holds the value the merge gave it                                                                         |
| Appearances and candidate pairs                           | Back to the source; the target's copy of a moved pair goes unless the owner decided it since                                                     |
| The pair of the source and the target                     | Marked `dismissed` in `face_candidates`, added when it has no row, so neither the cast rule nor the face-image rule merges it again or offers it |
| A smart folder rule's person                              | Back to the source while the rule still names the target                                                                                         |

Editing a video's credits writes no activity entry, so the owner's later
changes cannot be read from entry ids as auto-tag undo reads them. Instead every
work-information write (the details editor, the library selection, plugin
apply and `/api/v1`) marks, in its transaction, the recorded credits it removes
from the target, so a credit the owner removed and added again is the owner's
and stays. A name the owner gives the target is never touched. Undo of an entry
whose target was deleted, or whose source id exists again, records
`already_reverted` and changes nothing.

| Rejected                           | Why                                                                                                                |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Keep merged sources as hidden rows | Every read of people and credits would have to skip them                                                           |
| Compare the credits at undo        | A credit the owner removed and added again looks unchanged                                                         |
| Make every owner merge undoable    | A merge from the people list or the person page has no faces page to undo it from, and owner merges write no entry |
| Leave the undone pair open         | The work-information write of the undo, or the next analysis, would apply the same rule and merge the pair again   |

## Face entries on the activity screen

Both kinds belong to the `face_recognition` feature of the activity filter. A
`face_merge` entry's subject is the merge target as a `person` subject, whose
item carries `personId` from the subject key, so the card and the detail page
open `/people/:id`; its result names the source as `Unnamed person`, since a
merged source is always provisional and the entry keeps no number for it. A `face_analysis` entry lists its
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

| Action  | Effect                                                                                                                                                                                                                                                                                                                                   |
| ------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Confirm | The merge of the other person into the person it is confirmed from, in one transaction with the pair lookup; an unnamed source is merged as `MergeProvisional` does and recorded as a `face_merge` entry decided by the owner, whose id the response carries as `undoActivityId`, and a named source by the ordinary merge with no entry |
| Dismiss | Sets `dismissed`; a later result that offers the same pair updates only its similarity, so the pair stays hidden                                                                                                                                                                                                                         |
| Reopen  | Clears `dismissed` on a dismissed pair, for the faces page's `Undo`; the pair is offered again as before                                                                                                                                                                                                                                 |

The person confirmed from stays, with their face, whichever side the pair
stores as provisional; only when that person has no canonical name and the
other has one does it merge into the other, so a name is never lost. Without
an open pair, confirm and dismiss answer `404 face_candidate_not_found`, as
reopen does without a dismissed one.

| Rejected                                       | Why                                                                                                                                 |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| Pairs of appearances                           | The person page offers people, and one person's many appearances would repeat one choice                                            |
| Merging the stored provisional side, as before | Sorting unnamed people, the person on the page was often the one merged away, so the page and its face switched to the other person |

## The faces page reads a person's appearances

`GET /api/people/{id}/appearances` lists a person's appearances that are not
dropped, with the admin lists' `limit` and `cursor` and the `total` of them all,
each with its `similarity`: the cosine similarity of its centroid to the
normalized mean of the person's centroids of the same recognizer, which stands
for the person without needing a face image. `sort=leastSimilar` orders by that
similarity, lowest first, so faces linked by mistake come first; appearances
that cannot be compared follow in the default order
([`face_appearance_list.go`](../../internal/store/face_appearance_list.go)).
By default they come newest video first by the video's added time, then by
descending appearance id. The store reads every appearance of the person and
orders and pages them in memory, since the similarity needs all centroids; a
cursor names the last appearance by what its order compares, so a split face
leaving the list does not shift the next page. An appearance whose content key no video has any more is still
listed, without `videoId` and after every appearance with a video, so the
owner sees that the person was found in a file that has left the library.
Each item's spans are its tracks' times in order, with overlapping tracks
joined.

`GET /api/face-appearances/{id}/crop` answers the crop with
`Cache-Control: private, max-age=86400`: a crop is never rewritten, and a
re-analysis gives a new appearance a new id. A missing appearance and one whose
person was deleted, which has no crop, both answer `404 not_found`.

| Rejected                                   | Why                                                                                                    |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------ |
| Hiding appearances of content no video has | A returning file keeps its links, and the person page would show nothing for a person found only there |
| Ordering by appearance id alone            | A re-analysis keeps old ids, so the order would follow the first analysis, not the library             |

## The owner splits a wrongly linked face into its own person

Undoing a `face_analysis` link only drops the appearance, so a face linked to
the wrong person could not become a person of its own.
`POST /api/people/{id}/appearances/{appearanceId}/split` does it
([`face_split.go`](../../internal/store/face_split.go)) in one transaction and
answers `201` with the new person:

| Step       | Effect                                                                                                                                                                                                                                         |
| ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| New person | A provisional person whose face is the appearance's crop, made by the same `createFacePerson` the analysis uses; the face-image job embeds it later like any face                                                                              |
| Appearance | `person_id` moves to the new person; neither person's face vector changes, since a centroid is not one                                                                                                                                         |
| Credit     | The video's performer row of the old person becomes the new person's, in the same position; it stays with the old person, and the new person is appended, when another appearance of the old person in a video of the same work still links it |
| Pair       | The pair of the two people is marked `dismissed`, added when it has no row, so neither the cast rule nor the face-image rule merges them again and no candidate offers it                                                                      |

A re-analysis carries the appearance over by its tracks and keeps its
`person_id`, so it stays with the new person. A later video matches the new
person by their face, the crop, once the face worker embeds it. A person without that appearance, an appearance that is dropped and
one of another person answer `404`: `person_not_found` and
`face_appearance_not_found`. The split writes no activity entry; it is the
owner's own decision, like confirming a candidate.

`POST /api/people/{id}/appearances/split` does the same for several
appearances at once, moving them all to one new person whose face is the first
one's crop; any appearance the person lacks splits none.

| Rejected                                                       | Why                                                                                               |
| -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| Removing the old person's credit whenever the appearance moves | Another appearance in the same work may still be that person, and the credit would vanish with it |
| One new person per face for a selection                        | Faces wrongly linked together are usually one other person, who would then be several             |
| Copying the centroid into the new person's face-image vector   | A centroid is not the vector of the crop; the face-image job computes the right one               |

## Tests that cross the worker and the store live in `cmd/mdm`

The analysis of two videos of one person into one provisional person, a short
face that makes nobody, and a face image that merges the provisional person of
both videos cross the face worker and the store. `depguard` keeps
`internal/app` tests from importing `internal/store`, so these run in
[`cmd/mdm/face_analysis_test.go`](../../cmd/mdm/face_analysis_test.go) with the
real store, the event bus wiring and the worker, and with frames and the helper
faked per video. The rules each step applies are tested in their own package.
