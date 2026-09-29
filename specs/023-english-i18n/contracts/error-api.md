# Contract: API error reasons and failure codes

The source of truth is [api/openapi.yaml](../../../api/openapi.yaml); `task generate` produces the
Go and TypeScript code from it. This document lists only what this feature adds or changes. The
existing `code` values, the HTTP status of each route, and machine-readable values such as `state`
do not change.

## 0. Every `message` is English

`Error.message` is an English sentence for users and API clients. It does not contain free text
returned by the OS or by an external program. When it includes user data (a path, a name), the
data stays untranslated and only the surrounding explanation is English. The `description`
changes to "An English explanation. The UI renders from `code` and `reason`; this text is the
fallback for API clients and for unknown codes".

The plain-text failure responses in `internal/httpapi/spa.go` (`http.Error`) and the fixed JSON
that `writeJSON` sends when it cannot build a response are also English.

## 1. `reason`, `limit` and `tagName` on `Error`

`Error` gains three optional fields. `additionalProperties: false` stays.

| Field | Type | Meaning |
| --- | --- | --- |
| `reason` | string enum (`ErrorReason`) | A sub-reason that tells situations apart within one `code`. Returned only in the situations in the table below |
| `limit` | integer | The upper bound for that reason. Returned only when the `limit` column of the table has a value |
| `tagName` | string | The original name of the conflicting tag (user data, not translated). Returned only when the table marks `tagName` |

A response without `reason` behaves as today: the UI shows the text for the `code`.

| `code` (HTTP) | `reason` | `limit` | Situation (current location) |
| --- | --- | --- | --- |
| `tag_name_taken` (409) | `name_is_tag` | — | The submitted name is the original name of an existing tag. Returns `tagName` (`tagNameTakenMessage` in `tags.go`) |
| `tag_name_taken` (409) | `name_is_synonym` | — | The submitted name is a synonym of another tag. `tagName` is that tag's original name (same location) |
| `tag_merge_required` (409) | — | — | The rename target is the name of another tag. Returns `tagName` (`tags.go`) |
| `invalid_request` (400) | `username_length` | `domain.MaxUsernameLength` | Username length (`auth.go`) |
| `invalid_request` (400) | `password_length` | `domain.MaxPasswordBytes` | Password length (`auth.go`) |
| `invalid_request` (400) | `tag_name_empty` | — | Empty tag name (`domain.NormalizeTagName`; `tags.go`, `video_tags.go`, and tagging from a folder name in `folder_groups.go`) |
| `invalid_request` (400) | `tag_name_control_characters` | — | Control characters in the tag name (same locations) |
| `invalid_request` (400) | `tag_name_too_long` | `domain.TagNameMaxLength` | Tag name too long (same locations) |
| `invalid_request` (400) | `merge_same_tag` | — | Merge source and target are the same tag (`tags.go`) |
| `invalid_request` (400) | `search_too_long` | `maxQueryLength` | Search term too long (`videos.go`) |
| `invalid_request` (400) | `too_many_tag_filters` | `maxTagFilterCount` | Too many tag filters (`videos.go`) |
| `invalid_request` (400) | `too_many_videos` | `maxVideoTagsIDs` | Zero or too many videos in a bulk operation (`video_tags.go`, `visibility.go`) |
| `invalid_request` (400) | `guest_filter_not_allowed` | — | A guest used an owner-only filter (`videos.go`) |
| `invalid_request` (400) | `invalid_cursor` | — | The cursor cannot be parsed (`videos.go`, `library.go`, `folders.go`) |
| `invalid_request` (400) | `invalid_folder_path` | — | Invalid folder specification (`folders.go`) |
| `invalid_request` (400) | `relative_directory_path` | — | Relative path in the directory picker (`directories.go`) |
| `not_found` (404) | `video_not_found` | — | Video missing or not visible (`videos.go`, `reprobe.go`) |
| `not_found` (404) | `folder_not_found` | — | Folder missing or not visible (`folders.go`, `folder_groups.go`) |
| `not_found` (404) | `not_folder_group` | — | The folder is not a group (`library.go`) |
| `not_found` (404) | `no_scan` | — | No scan has run yet (`scans.go`) |
| `not_found` (404) | `directory_not_found` | — | The selected directory does not exist (`directories.go`) |
| `not_found` (404) | `file_unavailable` | — | The video file cannot be opened (`stream.go`, `transcode.go`) |
| `conflict` (409) | `media_folders_changed` | — | Another operation changed the media folders (`media_folders.go`) |
| `conflict` (409) | `root_group_not_taggable` | — | The group of a registered folder itself cannot become a tag (`folder_groups.go`) |
| `conflict` (409) | `folder_not_group` | — | The folder is not currently a group (`folder_groups.go`) |
| `conflict` (409) | `probe_info_missing` | — | Probe data needed for the seek preview or live transcoding is missing (`seek_thumbnail.go`, `transcode.go`) |
| `conflict` (409) | `seek_preview_generating` | — | The seek preview is being generated (`seek_thumbnail.go`) |
| `conflict` (409) | `transcode_unavailable` | — | Live transcoding is not possible (`transcode.go`) |
| `forbidden` (403) | `cross_origin` | — | A change that is not same-origin (`media_folders.go`) |
| `forbidden` (403) | `open_not_local` | — | An open request from a PC other than the server (`open.go`) |

Other `invalid_request` and `not_found` responses return no `reason`. These cover values the UI
does not send: sort order, `attempt`, Content-Type, a missing route, and similar.

- The server fills `limit` from its constants; the UI does not keep its own copy of the bounds.
- The conflicting tag name that the current Japanese `message` embeds is returned in `tagName`.
  The UI combines it with the submitted name into a specific explanation (the specificity of
  Requirement 6).
- A `domain.NormalizeTagName` failure becomes a domain error that carries a reason, instead of
  the current string. `internal/httpapi` maps it to `reason`.

## 2. `Video.probeErrorCode`

`Video` gains an optional `probeErrorCode` (string enum `ProbeErrorCode`). It is returned only for
rows with `probeState = failed` that have a stored code. The values are in the table in
[data-model.md §1](../data-model.md#1-videosprobe_error_code). `probeError` (free text) is still
returned as today. Guest responses omit both fields, the same as `probeError` today.

The UI does not show `probeError`. It shows the explanation for `probeErrorCode` when present, and
a generic English summary otherwise (rows from before the upgrade).

## 3. `Scan.errorCode` and `Scan.errorPath`

`Scan` gains two optional fields.

| Field | Type | Meaning |
| --- | --- | --- |
| `errorCode` | string enum (`ScanErrorCode`) | The reason when `state = failed` and a code is stored. Values in [data-model.md §2](../data-model.md#2-scanserror_code-and-scanserror_path) |
| `errorPath` | string | The absolute path when the reason is tied to a specific location: the media folder, or an unreadable location under it (user data, not translated) |

`error` (free text) is still returned as today. The UI does not show `error`; it builds the English
explanation from `errorCode` and `errorPath`. A `failed` scan without `errorCode` (a row from
before the upgrade) shows a generic English summary.
