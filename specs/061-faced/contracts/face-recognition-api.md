# Contract: Face recognition in the screen API

Source of truth: [`api/openapi.yaml`](../../../api/openapi.yaml). This
contract lists the operations this feature adds and the schemas it changes;
every new operation is owner-only. `api/external-v1.yaml` changes only by the
`provisional` property of its person schema.

## `GET /api/settings/face-recognition`

Returns the settings, the runtime state and the queue counts
([R-16](../research.md#r-16-download-progress-is-read-from-the-settings-resource)).

| Field | Type | Meaning |
| --- | --- | --- |
| `enabled` | boolean | `face.enabled` |
| `recognizer` | string | `sface_int8` or `arcface_r50` |
| `recognizers` | array | Per recognizer: `id`, `downloadBytes`, `license` (SPDX id or `non-commercial`), `nonCommercial` |
| `runtime.state` | string | `unsupported`, `absent`, `downloading`, `ready`, `failed` |
| `runtime.bytesDone`, `runtime.bytesTotal` | integer | Present while `downloading` |
| `runtime.reason` | string | Present when `failed`: `network`, `hash_mismatch`, `disk`, `helper_start` |
| `counts` | object | `queued`, `running`, `done`, `failed` content keys |
| `lastFailure` | string | The latest failed analysis's message, when one exists |

## `PUT /api/settings/face-recognition`

Saves `enabled` and `recognizer`. Turning it on starts the download of the
runtime and the recognizer's model when they are absent; turning it off
cancels a running analysis. A changed recognizer clears the vectors and
crops of the old one and queues every video
([R-8](../research.md#r-8-face-data-is-keyed-by-content-key-and-survives-a-model-switch-by-its-spans)).

| Status | `code` | When |
| --- | --- | --- |
| 400 | `invalid_request` | Unknown `recognizer` |

**Response**: `200` with the `GET` body.

## `POST /api/face-recognition/download`

Starts the download again after `failed`; `202` with the `GET` body.

| Status | `code` | When |
| --- | --- | --- |
| 409 | `face_recognition_off` | The feature is off |
| 409 | `face_recognition_unsupported` | No runtime is published for this platform |

## `POST /api/face-recognition/runs`

Queues videos for analysis: `{"scope": "missing" | "all"}`, as
`POST /api/auto-tagging/runs` does; answers `{"queued": n}`. Videos queued
while the runtime is not `ready` wait for it.

| Status | `code` | When |
| --- | --- | --- |
| 409 | `face_recognition_off` | The feature is off |

## `DELETE /api/face-recognition/data`

Deletes the face data
([R-15](../research.md#r-15-deleting-face-data-removes-vectors-and-face-crops-only));
`204`. Allowed while the feature is off.

## `GET /api/people/{id}/appearances`

The person's appearances, newest video first, paged by the admin lists'
cursor: each item has `appearanceId`, `videoId` (absent when no video has the
content key), the video's `title`, `spans` (`[{start, end}]` in seconds) and
`hasCrop`. Dropped appearances are not listed.

| Status | `code` | When |
| --- | --- | --- |
| 404 | `person_not_found` | No such person |

## `GET /api/face-appearances/{id}/crop`

The appearance's face crop as `image/jpeg`, `private, max-age=86400`, since a
crop never changes once written.

| Status | `code` | When |
| --- | --- | --- |
| 404 | `not_found` | No such appearance, or its crop was deleted |

## `GET /api/people/{id}/face-candidates`

Open (not dismissed) pairs in which the person takes part, highest similarity
first: each item has `person` (`CreditRef` with `provisional` and
`imageRevision`) and `similarity`.

## `POST /api/people/{id}/face-candidates/{otherId}/confirm`

Merges the provisional one of the pair into the other through the ordinary
merge, and returns the target `Person`. The owner's merge records no
`face_merge` entry
([R-11](../research.md#r-11-an-automatic-merge-keeps-what-it-moved-so-undo-can-move-it-back)).

| Status | `code` | When |
| --- | --- | --- |
| 404 | `face_candidate_not_found` | No open pair of these two people |

## `POST /api/people/{id}/face-candidates/{otherId}/dismiss`

Marks the pair dismissed; `204`. A later analysis or face image never offers
it again.

| Status | `code` | When |
| --- | --- | --- |
| 404 | `face_candidate_not_found` | No open pair of these two people |

## Changed schemas and operations

| Item | Change |
| --- | --- |
| `Person`, `CreditRef`, `WorkPerson` | Required `provisional: boolean`; `name` is `""` when it is true ([R-7](../research.md#r-7-a-provisional-person-is-a-person-without-a-canonical-name)) |
| `GET /api/people` | Query `provisional=true|false`; `400 invalid_request` on an organization list; name sorts put provisional people last |
| `PATCH /api/people/{id}` | A `name` on a provisional person makes it an ordinary person |
| `PUT /api/people/{id}/image` | `400 person_image_no_face` and `400 person_image_many_faces`, only while the runtime is `ready` ([R-12](../research.md#r-12-a-face-image-is-embedded-after-it-is-stored)) |
| `ActivityEntry` | Kinds `face_analysis` and `face_merge`, feature `face_recognition`, subject kind `person`; a `face_analysis` entry lists `faceLinks` (`appearanceId`, `person`, `createdPerson`, `undone`) |
| `ActivityUndoRequest` | Optional `appearanceId` undoes one link of a `face_analysis` entry, as `tagId` does for `auto_tag`; without it, every link not yet undone |
| `POST /api/activity/{id}/retry` | Accepts `face_analysis` entries in `attention`; `409 face_recognition_off` while off |
