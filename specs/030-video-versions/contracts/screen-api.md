# Contract: Screen API for versions and bundling

Source of truth: `api/openapi.yaml`. This document holds only the new routes and the `Video` delta. The
decisions are [research.md R-8](../research.md) and [R-9](../research.md). The error shape is
[specs/023-english-i18n/contracts/error-api.md](../../023-english-i18n/contracts/error-api.md); guest handling is
[specs/016-single-account-auth/contracts/guest-api.md](../../016-single-account-auth/contracts/guest-api.md).

## 0. `Video` delta

| Field | Type | Rule |
| --- | --- | --- |
| `versions` | `VideoVersionsRef {count, representativeId}` | Only for bundle members, and only in the `GET /api/videos/{id}` response (treated like `group`). `count` is the number of versions with a location the viewer may see; `representativeId` is the effective representative's id |
| `tags`, `progress`, `public` | Existing | For bundle members, the bundle's values ([data-model.md §3](../data-model.md)). What is omitted for owners and guests does not change |

The lists (`GET /api/library`, `GET /api/videos`, `GET /api/folders/{rootId}/videos`) and the folder list never
show non-representative versions ([data-model.md §4](../data-model.md)). The same applies to
`GET /api/library/ids`. `GET /api/videos/{id}`, `related`, `stream`, `transcode.mp4`, `subtitles`, `thumbnail`,
`seek-thumbnail` and `preview` work unchanged on non-representative versions.

## 1. `GET /api/videos/{id}/versions`

`operationId: listVideoVersions`. Guests allowed (same class as `GET /api/videos/{id}`).

| Case | Response |
| --- | --- |
| The video can be shown | `200` `VideoVersions {representativeId, items: Video[]}`. Representative first, then by natural title order. For a video in no bundle, `items` is that one video |
| Missing, or cannot be shown | `404 not_found` / `video_not_found` |

Each `Video` in `items` has the same shape as `GET /api/videos/{id}` (`location` for owners, `folder` for both)
and carries resolution, codec, `sizeBytes` and `container`.

## 2. `POST /api/video-bundles`

`operationId: bundleVideos`. Owner only. Body: `{ "videoIds": int64[], "representativeId": int64 }`.

| Case | Response |
| --- | --- |
| Bundled | `200` `VideoVersions` (the new bundle) |
| `videoIds` has fewer than 2 (duplicates count once), or exceeds the limit (same as `POST /api/video-tags`) | `400 invalid_request` / `too_few_videos`, `too_many_videos` |
| `representativeId` is not in `videoIds` | `400 invalid_request` / `representative_not_selected` |
| Some id is missing, or has no location under a media folder | `404 not_found` / `video_not_found`. Nothing changes |
| Guest | `401 unauthenticated` |

When a video already in a bundle is included, every member of that bundle joins the new bundle. If this pair is
a candidate, the candidate is removed. After commit, `video` is sent on `/api/events` for every member.

## 3. `POST /api/videos/{id}/make-representative`

`operationId: makeRepresentativeVersion`. Owner only. No body.

| Case | Response |
| --- | --- |
| Made representative (same when it already is) | `200` `VideoVersions` |
| In no bundle | `400 invalid_request` / `not_bundled` |
| Missing, or has no location under a media folder | `404 not_found` / `video_not_found` |

The bundle's tags, playback position and visibility do not change (requirement 7).

## 4. `POST /api/videos/{id}/unbundle`

`operationId: unbundleVideo`. Owner only. No body.

| Case | Response |
| --- | --- |
| Removed | `200` `Video` (the removed video; `tags`, `progress` and `public` are its own values from before bundling, no `versions`) |
| In no bundle | `400 invalid_request` / `not_bundled` |
| Missing, or has no location under a media folder | `404 not_found` / `video_not_found` |

If one member remains, the bundle is dissolved and that video holds the bundle's values (Edge Case). `video` is
sent for every former member.

## 5. Candidates

### `GET /api/version-candidates`

`operationId: listVersionCandidates`. Owner only. No parameters.

`200` `VersionCandidatePage { items: VersionCandidate[], total }`. `VersionCandidate` is
`{ videos: [Video, Video], distance }` (`videos` in ascending id order; each `Video` has the same shape as
`GET /api/videos/{id}`). Newest first, at most 200. `total` counts all.

### `POST /api/version-candidates/dismiss`

`operationId: dismissVersionCandidate`. Owner only. Body: `{ "videoIds": [int64, int64] }`.

| Case | Response |
| --- | --- |
| Recorded as "different videos" (recorded even when the pair is not a candidate) | `204` |
| Not exactly 2 ids, or the same id twice | `400 invalid_request` |
| Either is missing, or has no location under a media folder | `404 not_found` / `video_not_found` |

"Same video" passes the two videos and the representative to `POST /api/video-bundles`.

## 6. `/api/events`

| Kind | When |
| --- | --- |
| `video` | Bundling, changing the representative, removing, and scan-time carry-over send the ids of every affected member ([R-9](../research.md)) |
| `scan` | Sent on success or failure of a `fingerprint` job. The candidate screen refetches on it. No new kind is added |

## 7. What does not change

- `api/external-v1.yaml` ([R-10](../research.md)). `tags` and `POST /api/v1/video-tags` hold the bundle's values.
- The schemas of list items, `LibraryGroup` and `RelatedVideos`.
