# Contract: External API changes

Parent Issue: #630.

Source of truth: [api/external-v1.yaml](../../../api/external-v1.yaml). This file
lists only the added fields. `task generate` regenerates
`internal/httpapi/extgen/`. The external API's compatibility policy (fields are
only added) is in
[specs/026-external-api/contracts/external-api.md](../../026-external-api/contracts/external-api.md).

## 0. Fields added to `ExternalVideo`

```yaml
ExternalVideo:
  required: [..., addedAt, updatedAt, fileCreatedAt, locations, tags]
  properties:
    updatedAt:
      type: string
      format: date-time
      description: |
        When the video's information (display name, tags, visibility, thumbnail) was last
        edited in vv. Equal to addedAt if never edited. This API's video-tags and
        display-names also advance it
    fileCreatedAt:
      type: string
      format: date-time
      description: |
        Creation time of the file at the representative location (the first of locations).
        The file's modification time (mtime) when the file system does not provide one
```

The fields appear in `GET /api/v1/videos`, `GET /api/v1/videos/lookup`, and every
other response that returns `ExternalVideo`. The conversion in
`internal/httpapi/external_videos.go` is `item.Video.EditedAt.UTC()` and
`item.Video.FileCreatedAt.UTC()` (handled like `addedAt`).

## 1. What does not change

- The list order (`addedAt`, then `id`, ascending) and its cursor, `limit`, and
  how `lookup` resolves a video.
- The request and response shapes of `POST /api/v1/video-tags` and
  `display-names`. The edit time advances by the rules in
  [data-model.md §3](../data-model.md#3-edit-time-rules).
- MCP tools (`list_videos`, `lookup_video` and others) return the same handlers'
  responses as is, so their structured output gains the two fields and the tool
  definitions do not change.
- The field descriptions in the section on listing videos (`動画の一覧を読む`) of
  `docs/how-to/external-api.md` gain the two fields.
