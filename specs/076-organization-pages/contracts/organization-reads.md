# Contract: Organization children and the organization role filter

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml), operations
`listOrganizationChildren` (new) and `listVideos` (changed). Both are
owner-only, like every organization read.

## `GET /api/organizations/{id}/children`

Lists the organizations whose parent is `{id}`, one level down.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `id` | path | integer | yes | The parent organization (`OrganizationId`) |

**Response**: `200` with an `OrganizationList`: every direct child, ordered by
the name sort key then id, `total` and `totalAll` equal to the item count, no
`nextCursor`. An organization without children answers an empty list.

| Status | `code`, `reason` | When |
| --- | --- | --- |
| 404 | `not_found`, `organization_not_found`, no `field` | No organization has `{id}` |
| As every owner-only operation | — | The viewer is a guest; the operation is not added to the guest routes in `internal/httpapi/auth.go` |

A remote id is forwarded to its link by the existing boundary. Its
`OrganizationList` answer is rewritten item by item as a forwarded
`Organization` is: a child takes the same-named local id, else the offset form
([peer-library.md](../../../docs/design-docs/peer-library.md)).

## `GET /api/videos`: `organizationRole`

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `organizationRole` | query | `WorkOrganizationRole` (`maker`, `label`) | no | Keeps only videos linked to `organization` (or `organizationName`) in this role, including `total` |

**Response**: Unchanged.

| Status | `code` | When |
| --- | --- | --- |
| 400 | `invalid_request` | `organizationRole` without `organization` or `organizationName`, or an unknown role |
| 400 | `invalid_request`, reason `guest_filter_not_allowed` | The viewer is a guest |

On a paired node the parameter goes wherever the organization filter goes, as
`personRole` does: to each link with `organizationName` for a local
organization, and with `organization` to its own link only for a remote one.

## Client use

The organization page reads the children once per page load, and each role
shelf reads `GET /api/videos?organization={id}&organizationRole={role}` in the
library's default order, a page at a time, as the person page's shelves do.
