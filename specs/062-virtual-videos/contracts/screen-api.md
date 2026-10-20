# Contract: Screen API for virtual videos

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml). This file lists
only what the feature adds or changes; [api/external-v1.yaml](../../../api/external-v1.yaml)
and MCP do not change ([research R-4](../research.md#r-4-visibility-gains-a-virtual-branch)).

## `Video` schema

| Field | Change |
| --- | --- |
| `virtual` | New, required boolean |
| `contentVersion` | The whole user key for a virtual video ([research R-6](../research.md#r-6-optimistic-check-by-contentversion)) |
| `workInfo` | Sent to guests too, carrying only `description` and `urls` ([research R-5](../research.md#r-5-guests-read-description-and-links)) |
| `sizeBytes` | `0` for a virtual video |
| `fileCreatedAt` | The creation time for a virtual video |
| `playable` | `false` for a virtual video, with no `unplayableReason` |
| `location`, `folder`, `fileTitle`, `durationMs`, media fields, `previewUrl`, `seekThumbnailUrl`, `versions`, `mediaInfo` | Absent for a virtual video |

## `POST /api/virtual-videos`

Creates a virtual video; owner only.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `title` | body | string | Yes | Not empty after trimming |

**Response**: `201` with the `Video`.

| Status | `code` | When |
| --- | --- | --- |
| 400 | `invalid_title` | The title is empty or whitespace only |

## `DELETE /api/videos/{id}`

Deletes a virtual video and its user data; owner only.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `contentVersion` | query | string | Yes | The version the screen read |

**Response**: `204`.

| Status | `code` | When |
| --- | --- | --- |
| 404 | `not_found` | No such video, including one already deleted or merged |
| 409 | `content_changed` | The version does not match |
| 409 | `not_virtual` | The video has a file |

## `PUT /api/videos/{id}/details`

Adds an optional `contentVersion` to `VideoDetailsInput`. For a virtual video
it is required, and `displayName` sets the title, so an empty value answers
`400 invalid_title` instead of clearing it.

| Status | `code` | When |
| --- | --- | --- |
| 404 | `not_found` | The virtual video was deleted or merged |
| 409 | `content_changed` | The version does not match |

`PUT /api/videos/{id}/display-name` follows the same rules for a virtual video.

## `POST /api/videos/{id}/thumbnail-image`

Accepts a virtual video. The image is fitted to 16:9
([research R-7](../research.md#r-7-thumbnails-from-an-uploaded-image-only)); `DELETE` removes it.
`PUT /api/videos/{id}/thumbnail-position` and the thumbnail-edit endpoints
answer `409 virtual_video`.

## `POST /api/virtual-videos/{id}/merge`

Merges the virtual video into a file video; owner only
([research R-8](../research.md#r-8-merge-moves-user-data-into-the-file-videos-user-key)).

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `id` | path | integer | Yes | The virtual video |
| `contentVersion` | body | string | Yes | The virtual video's version the screen read |
| `videoId` | body | integer | Yes | The file video |
| `videoContentVersion` | body | string | Yes | The file video's version the screen read |

**Response**: `200` with the file video's `Video`.

| Status | `code` | When |
| --- | --- | --- |
| 404 | `not_found` | Either video is missing or hidden |
| 409 | `content_changed` | Either version does not match |
| 409 | `not_virtual` | `id` has a file |
| 409 | `virtual_video` | `videoId` is a virtual video |
| 409 | `description_too_long` | The combined description exceeds the work-information limit |
| 409 | `too_many_urls` | The combined links exceed the work-information limit |

## `GET /api/videos`

Adds the query parameter `kind` (`all`, default; `file`; `virtual`), so the
merge picker lists file videos only.

## `GET /api/playlists/{id}/playback`

`PlaylistPlayback.next`, the continuous-playback step, skips virtual items;
`previousItem` and `nextItem`, the manual steps, do not. A new required
`playableCount` gives the number of visible items with a file, so the playlist
screen and the player show that nothing can be played when it is `0`
([research R-9](../research.md#r-9-playlist-playback-skips-virtual-videos-on-the-server)).

## Version-bundle operations

`POST /api/video-bundles`, `make-representative` and `unbundle` answer
`409 virtual_video` when any video in the request is virtual
([research R-10](../research.md#r-10-virtual-videos-stay-out-of-version-bundles)).
