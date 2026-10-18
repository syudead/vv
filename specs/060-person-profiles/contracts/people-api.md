# Contract: people in the screen API and the external API

Source of truth: [`api/openapi.yaml`](../../../api/openapi.yaml) (`listPeople`,
`createPerson`, `getPerson`, `updatePerson`, `mergePeople`, `listVideos`, and
the `Person`, `CreditInput`, `CreditPatch`, `CreditSort` and `WorkPerson`
schemas) and [`api/external-v1.yaml`](../../../api/external-v1.yaml) (`Person`,
`CreditInput`, the people operations). The MCP people tools take their input
schemas from the external types, so they follow without a schema of their own;
their hand-written descriptions name the profile.

## `Person` and `PersonProfile`

`Person` gains `profile` (always present, each property optional) and
`imageRevision` (present only when the person has a face). The external
`Person` gains `profile` only.

| `PersonProfile` property | Type | Meaning |
| --- | --- | --- |
| `gender` | `female` \| `male` \| `other` | |
| `birthDate`, `deathDate` | string | `YYYY`, `YYYY-MM` or `YYYY-MM-DD` |
| `birthplace` | string | |
| `heightCm` | integer | |
| `bustCm`, `waistCm`, `hipsCm` | integer | |
| `cup` | string | |
| `bloodType` | `A` \| `B` \| `O` \| `AB` | |
| `careerStartYear`, `careerEndYear` | integer | |
| `bio` | string | |
| `links` | string[] | URLs in order; absent when empty |

Ranges and formats are in [data-model.md, Rules](../data-model.md#rules).

## `POST /api/people`, `PATCH /api/people/{id}` and the external equivalents

`CreditInput` and `CreditPatch` gain an optional `profile`. In `CreditInput`
it is the whole profile. In `CreditPatch` each property present replaces that
field, `null` clears it, and `links` replaces the list. A profile-only patch
that changes nothing returns the current person without writing.

| Status | `code` | When |
| --- | --- | --- |
| 400 | `invalid_request` | `profile` sent for an organization |
| 400 | `work_info_*` with `field` `profile.<name>` | A value out of range, a bad date, a death date before the birth date, an end year before the start year |
| 400 | `work_info_too_many` with `field` `profile.links` | More than 20 links |

## `GET /api/people` and `GET /api/v1/people`

`CreditSort` gains `ageAsc` and `ageDesc`
([R-4](../research.md#r-4-age-sort-is-a-birth-date-sort-with-the-unknown-last)).

| Status | `code` | When |
| --- | --- | --- |
| 400 | `invalid_request` | `ageAsc` or `ageDesc` on an organizations list |

## `POST /api/people/{id}/merge`

Unchanged request and errors. The merged person's profile, links and face
follow [R-8](../research.md#r-8-merge-fills-the-targets-empty-fields-from-the-sources).

## `GET /api/people/{id}/image`

Owner-only. Returns the face as `image/jpeg` with `Cache-Control: private,
no-cache` and the revision as ETag; a matching `If-None-Match` returns `304`.
The `v` query parameter is ignored by the server and only busts caches.

| Status | `code` | When |
| --- | --- | --- |
| 404 | `person_not_found` | No such person |
| 404 | `person_image_not_found` | The person has no face |

## `PUT /api/people/{id}/image`

Owner-only. Takes one `multipart/form-data` part `image` (PNG, JPEG or still
WebP), fits and stores it
([R-5](../research.md#r-5-a-face-is-one-jpeg-blob-in-sqlite-fitted-on-the-server-in-pure-go)),
and returns the updated `Person`. The limits are the thumbnail upload's.

| Status | `code` | When |
| --- | --- | --- |
| 400 | `person_image_invalid` | Not a decodable PNG, JPEG or still WebP, or over the pixel limit |
| 413 | `person_image_too_large` | Body over the byte limit; carries the limit |
| 404 | `person_not_found` | No such person |

## `DELETE /api/people/{id}/image`

Owner-only. Removes the face and returns the updated `Person`; a person
without a face is returned unchanged.

| Status | `code` | When |
| --- | --- | --- |
| 404 | `person_not_found` | No such person |

## `WorkPerson`

The video's work information gains `imageRevision` on each `WorkPerson` whose
person has a face, so the cast loads `/api/people/{id}/image?v=…`.

## `GET /api/videos`

Gains the owner-only `personRole` (`WorkPersonRole`)
([R-11](../research.md#r-11-the-person-page-lists-videos-by-person-and-role-on-the-server)).
It keeps only videos linked to `person` in that role, and `total` counts them.

| Status | `code` | When |
| --- | --- | --- |
| 400 | `invalid_request` | `personRole` without `person`, or an unknown role |
| 400 | `invalid_request` with reason `guest_filter_not_allowed` | A guest sends `personRole`, as for `person` |
