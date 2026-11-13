# Contract: image search results with frames and bands

Source of truth: [api/openapi.yaml](../../../api/openapi.yaml), operations
`searchByImage` and `getVideoFrame`. This file holds only what the revised
results view changes; the request, the limits, the refusals and every field
not named here stay as the schema has them.

## `POST /api/image-search`

The answer gains the match-rate bands of the query's kind, and each match
gains its band and its matched frames.

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `ImageSearchResult.bands` | array of `ImageSearchBand` | Yes | Every band of the query's kind, strongest first, whether or not a match is in it |
| `ImageSearchBand.id` | `strong` or `similar` | Yes | The screen holds one name per id |
| `ImageSearchBand.minPercent`, `maxPercent` | integer, 0 to 100 | Yes | The whole-percent rates the band holds, both ends included |
| `ImageSearchMatch.band` | `strong` or `similar` | Yes | The band of `rate` rounded to the nearest whole percent, halves up |
| `ImageSearchMatch.frames` | array of `ImageSearchMatchFrame`, at most 12 | Yes | Image query: the matched frames of `videoId`, best first, at least one. Video query: empty |
| `ImageSearchMatch.frameCount` | integer | Yes | The matched frames found before the limit of 12; the length of `frames` when 12 or fewer were found |
| `ImageSearchMatchFrame.positionMs` | integer | Yes | The frame's time on `videoId`'s timeline |
| `ImageSearchMatchFrame.rate` | number, 0 to 1 | Yes | `(63 − hash distance) / 63` of that frame |

| Query kind | `bands` |
| --- | --- |
| Image | `strong` 90 to 100, `similar` 81 to 89 |
| Video | `strong` 90 to 100, `similar` 50 to 89 |

Rules the client relies on:

- `frames[0]` has the match's `rate` and `positionMs`.
- `frames` are ordered by rate, then the earlier time.
- `items` keep their order, so the matches of one band are next to each
  other and the bands appear in the order of `bands`.
- Which videos are listed, their `rate`, `kind`, `positionMs` and section
  fields, and the order of `items` do not change.

## `GET /api/videos/{id}/frame`

The operation gains an optional size, so a strip of matched frames does not
load full-size frames.

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `maxEdge` | query | integer, 64 to 2048 | No | The long edge's cap in pixels, never enlarged; 2048 when left out, as today |

| Status | `code` | When |
| --- | --- | --- |
| 400 | `invalid_request` | `maxEdge` is outside 64 to 2048 |

## Client use

`web/src/api/imageSearch.ts` builds a frame's picture as
`/api/videos/{match.videoId}/frame?positionMs={frame.positionMs}&maxEdge=320`
and its player link as `/videos/{match.videoId}?t=` at the frame's time.
