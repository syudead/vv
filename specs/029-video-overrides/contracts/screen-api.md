# Contract: Display name and thumbnail position in the screen API

Source of truth: `api/openapi.yaml`, operations `setVideoDisplayName` and
`setVideoThumbnailPosition`. This document covers only the two added routes
and the changes to `Video`. The decisions are in
[research.md R-8](../research.md#r-8-the-screen-api-is-two-puts-per-video-an-empty-display-name-and-a-null-position-clear),
[R-10](../research.md#r-10-display-name-rules-follow-tag-name-rules-with-a-200-code-point-limit)
and
[R-11](../research.md#r-11-position-validation-is-a-pure-function-in-internaldomain-with-three-distinct-errors).

## 0. `Video` changes

| Field | Type | Rule |
| --- | --- | --- |
| `title` | string (existing) | The effective title: the display name if there is one, otherwise the file name without its extension. Its description is rewritten |
| `fileTitle` | string | The file name without its extension. Only in owner responses |
| `displayName` | string | The display name. Only when set, and only in owner responses |
| `thumbnailPositionMs` | integer (int64) | The representative thumbnail position. Only when set, and only in owner responses |
| `thumbnailUrl` | string (existing) | When a position is set, the version is `<content key prefix>-r<revision>`. The position value is not included ([R-6](../research.md#r-6-the-thumbnailurl-version-includes-the-position-revision)) |

Guest responses receive the display name in `title` (requirement 1) and omit
the three new fields. `thumbnailUrl` is the same string as for the owner and
does not contain the position value (the same handling as `location` in
[specs/016-single-account-auth/contracts/guest-api.md §1](../../016-single-account-auth/contracts/guest-api.md)).
The `title` in lists, related videos, library items and `RelatedVideo` is also
the effective title.

## 1. `PUT /api/videos/{id}/display-name`

`operationId: setVideoDisplayName`. Owner only (`security: sessionCookie`; the
default category in `accessRoutes`). The body is `{ "displayName": string }`
(`required`, `Content-Type: application/json`).

| Situation | Response |
| --- | --- |
| Saved, or cleared because the value was empty after trimming | `200` `Video` (same shape as `GET /api/videos/{id}`, with `title`, `fileTitle` and `displayName` updated) |
| Contains control characters | `400 invalid_request` / `display_name_control_characters` |
| Longer than 200 code points | `400 invalid_request` / `display_name_too_long`, `limit: 200` |
| The video does not exist, or has no location under the media folders | `404 not_found` / `video_not_found` |
| Guest | `401 unauthenticated` (returned by the boundary; requirement 9) |

A display name equal to another video's is allowed (edge case). The last
committed transaction wins (concurrent changes: last write wins). After commit,
`video` for that video is sent on `/api/events`.

## 2. `PUT /api/videos/{id}/thumbnail-position`

`operationId: setVideoThumbnailPosition`. Owner only. The body is
`{ "positionMs": integer | null }` (`required`; `null` clears).

| Situation | Response |
| --- | --- |
| The image at the chosen position was made, published and recorded | `200` `Video` (`thumbnailState: done`, a new `thumbnailUrl` version, `thumbnailPositionMs`) |
| `null`, and the image at the automatic position was regenerated and recorded | `200` `Video` (no `thumbnailPositionMs`) |
| Analysis has not finished, or the duration is unknown | `409 conflict` / `duration_unknown` |
| `positionMs < 0` or at least the duration | `400 invalid_request` / `thumbnail_position_out_of_range`, `limit` = duration (milliseconds) |
| No image could be made (ffmpeg failed, 0 frames at the position) | `409 conflict` / `thumbnail_frame_unavailable`. The previous image and position are kept |
| No location opens | `404 not_found` / `file_unavailable` (same as `getVideoStream`) |
| The video does not exist | `404 not_found` / `video_not_found` |
| Guest | `401 unauthenticated` |

The response returns after generation finishes (this can take seconds).
Concurrent choices for the same video are serialized by the generation lock,
and the image of the last recorded position remains. After commit, a `video`
notification is sent and list cards read the new `thumbnailUrl`.

## 3. Added `code` and `reason` values

Nothing is added to `Error.code` (`conflict`, `invalid_request` and
`not_found` are used). `ErrorReason` gets `display_name_control_characters`,
`display_name_too_long`, `duration_unknown`, `thumbnail_position_out_of_range`
and `thumbnail_frame_unavailable`, and `web/src/i18n/errors.ts` gets an English
message for each.
