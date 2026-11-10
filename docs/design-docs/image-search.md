# Image search: finding footage by an image or video

Image search lists the owner's videos that contain the footage of one uploaded
image or video, with a match rate and the matched frame or section. The index
is built by the `frame_index` job, the rules are pure functions in
[`internal/domain/image_search.go`](../../internal/domain/image_search.go), and
the request runs in `ImageSearch` ([`internal/app`](../../internal/app/image_search.go)).

The diagram shows the parts a search touches and what each one reads.

```mermaid
flowchart LR
  page[Library page] -->|POST /api/image-search| http[HTTP layer]
  http --> search[ImageSearch]
  search -->|video query| uploads[Search uploads]
  search -->|hash the query| media[ffmpeg and hashing]
  search -->|index snapshot| store[Store]
  search -->|match| domain[Domain rules]
  job[frame_index job] -->|keyframe hashes| store
```

## Search index

Every video content with a file has one `video_search_index` row: the time
and hash of each keyframe, at least 1 s apart and at most 20,000, hashed by
the fingerprint's rule (black bands trimmed, 32×32 luma, `domain.HashFrame`),
with flat frames left out.

The 81-frame fingerprint used for duplicates is up to 1/81 of a video apart
(44 s in an hour), so a screenshot of an arbitrary moment rarely has a frame of
its scene there. Keyframes are read in one sequential pass with
`-skip_frame nokey`, which costs one read per file instead of a full decode.
The index is versioned apart from the fingerprint, so changing one never
rereads every file for the other ([job-scheduling.md](job-scheduling.md#each-kind-declares-a-demand)).

| Rejected | Why |
| --- | --- |
| Reuse the fingerprint | Its frames are too far apart for an arbitrary screenshot |
| Decode every frame and keep one per second | Minutes of CPU per hour of 1080p H.264, days over a library |
| Seek to each keyframe from the packet index | One random read per keyframe; a sequential pass reads less on spinning disks |

## Image queries

An image matches a content at its nearest indexed keyframe when the hashes
differ in at most 12 of 63 bits (`FingerprintMatchMaxDistance`), and the match
rate is `(63 − distance) / 63`.

Two unrelated hashes are within 12 bits once in 2.7 million comparisons but
within 18 once in 2,200, which over millions of entries would list thousands of
unrelated videos. A single image has one frame, so a share of matching frames
would make every match 100%. PNG, JPEG and still WebP are decoded in memory
with the thumbnail upload's decoder and limits; any other file is read as a
video.

## Video queries

A video query is sampled once a second, at most 10,800 frames, and the index's
keyframes are compared with the query frame nearest them under one time
offset.

```mermaid
flowchart LR
  seeds[One seed per 10 s, 64 to 512] --> votes[Votes per content and 2 s offset]
  votes -->|two votes| verify[Compare the whole overlap]
  verify --> rate{50% of at least 3 entries?}
  rate -->|yes| kind{Covers 90% of both?}
  rate -->|no| drop[Not listed]
  kind -->|yes| whole[Whole match]
  kind -->|no| partial[Partial match]
```

With the query sampled once a second, every indexed keyframe has a query frame
within 0.5 s whatever the two files' keyframe positions. Seeds keep a search
to seconds where comparing every query frame with every entry takes minutes.
Each entry stands for the span up to the next one, so a whole re-encode whose
keyframes are 8 s apart still covers the whole video; playback starts at the
first matching entry.

| Rejected | Why |
| --- | --- |
| The versions rule (at most 5% of frames differ) | A partial copy with a logo or subtitles differs in more frames and is still the same footage |
| Equal durations for a whole match | A re-encode trims or pads a few seconds at the ends |

## Results

Version bundle members collapse to the listed representative, which carries
the best member's match; the match names the member it was found in, and
playback opens that member at the match position. Groups are not collapsed.
Results are ordered by match rate, then matched section length, then the newer
video, at most 50, and come with the count of listed file videos, bundle
members included, that have no current index.

Versions can be shifted by seconds, so the position belongs to the member that
matched. The search reads the index under the owner's audience, so videos in
switched-off media folders are neither searched nor counted. The matched frame
is shown with the existing `GET /api/videos/{id}/frame`.

## Uploads leave nothing behind

An image query stays in memory; a video query is written to
`MDM_DATA_DIR/search-uploads/` ([`internal/artifacts`](../../internal/artifacts/search_uploads.go))
and removed when the request ends, whether it succeeded, failed or was
cancelled, and startup empties the directory before the listener opens.

An MP4 with its index at the end cannot be read from a pipe, and in a container
the OS temporary directory is often a small tmpfs. A cancelled request ends
ffmpeg and writes no response. One search runs at a time and a second request
waits until the first ends or the second is cancelled, because each reads the
whole index and may decode a three-hour video.

| Rejected | Why |
| --- | --- |
| A search job with polling | Nothing may outlive the request, and the screen shows only that the search runs |
| Searches in parallel | The client cancels its previous request before sending the next |
