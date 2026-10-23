# Contract: Screen API additions for the video page tabs

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). This feature
adds one operation, `listVideoFaceAppearances`; every other tab reads what the
page already reads (`getVideo`, `listVideoVersions`, `listWorkInfoPlugins`)
and sends what the information rows send today.

## `GET /api/videos/{id}/face-appearances`

Lists the people face recognition found in the video, with when each is on
screen ([R-3](../research.md#r-3-a-per-video-read-of-face-appearances)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `id` | path | integer | yes | Video ID |

**Response**: `200` with `VideoFaceAppearanceList`:

| Field | Type | Meaning |
| --- | --- | --- |
| `analysed` | boolean | True when the video's content key has a `face_queue` row in state `done`; false otherwise, including while queued or running and after a failure |
| `items` | array of `VideoFaceAppearance` | The not-dropped appearances of the content key, in order of their first span's start, then by appearance ID; `[]` when `analysed` is false |
| `items[].appearanceId` | integer | Appearance ID, for `GET /api/face-appearances/{id}/crop` |
| `items[].person` | `CreditRef` | The linked person; `name` is `""` and `provisional` true for an unnamed person |
| `items[].imageRevision` | integer, optional | The person's face image revision, as `WorkPerson.imageRevision`; absent without a face image |
| `items[].hasCrop` | boolean | As `FaceAppearance.hasCrop` |
| `items[].spans` | array of `FaceAppearanceSpan` | As `FaceAppearance.spans`: seconds, in order, without overlaps |

| Status | `code` | When |
| --- | --- | --- |
| `401` | `unauthenticated` | No owner session; the operation is owner-only (the global `security` default) |
| `404` | `video_not_found` | No video with the ID |

## Client use

The People panel reads it when it is selected and after `getVideo` replaces
the video, and shows the lower section only when `analysed` is true and
`items` is not empty; a person with `imageRevision` shows the person image
through `personImagePath`, otherwise the placeholder face.
