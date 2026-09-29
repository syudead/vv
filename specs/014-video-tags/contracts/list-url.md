# Contract: URL for the tag filter

Parent Issue: #193.

The source of truth for the library list (`/`) URL conditions is #195's
[list-url.md](../../013-library-search/contracts/list-url.md). This document covers only the
parameter this feature adds and how its handling differs. The folder page URL does not change.

## 1. Parameters

| Name | Value | When omitted |
| --- | --- | --- |
| `tag` | A tag `id`. One per selected tag (`?tag=3&tag=8`). At most 16 | No tag filter |

- The URL carries the `id`, not the name. A URL keeps the same filter after a rename. Choosing a
  tag by a synonym also yields the original tag's `id`, so one condition never has two URLs
  (Requirement 7).
- The values are written in ascending `id` order. The URL then does not depend on the selection
  order, and the keys of [listSnapshot](../../../web/src/api/listSnapshot.ts) keep matching.
- Non-numeric values, duplicates and values after the 16th are dropped. No error is shown.
- Every `id` in the list response's `missingTagIds`
  ([tags-api.md §5](tags-api.md#5-list-filter-and-select-all)) is removed from the URL. The
  current history entry is replaced; no entry is added. At the same time, the screen reports that
  the tag no longer exists and refetches the tag list (Edge case "Concurrent changes on other
  screens"). The list itself is shown as the server returned it, with the missing `id` ignored.
- The key of the list snapshot (`listSnapshot`) includes the normalized `tag` sequence. Going back
  from `/?tag=1` then does not reuse the snapshot of `/`.
- When the list is restored from the snapshot, no list request is made, so `missingTagIds` does
  not arrive. The screen instead compares the URL's `tag` with the shared tag list it refetches on
  restore. If an `id` is not in that list, the screen reports it and removes it from the URL, as
  for `missingTagIds`. The condition changes, so the snapshot is not used and the list is
  refetched.

## 2. Pressing and removing a tag

- Pressing a tag on a library list card adds its `id` to the current URL's `tag`. The search
  term (`q`), watch status, playability and sort order stay (Requirement 5). If the `id` is
  already in `tag`, nothing changes. When 16 tags are already set, the press adds nothing and
  the screen reports that no more can be added.
- Pressing a tag on the playback page, or choosing a tag on the management page, opens
  `/?tag=<id>` with no other conditions (Acceptance criteria 7 and 16). The sort order is the one
  saved on the device, per #195's rules.
- Removing an active tag above the list removes only its `id` from the URL's `tag`. Other
  conditions stay.
- Each tag addition or removal adds one history entry, as #195 does for watch status.
- #195's "Clear filters" also clears `tag`. #195's `ui-design.md` shows "Clear filters" only when
  watch status, playability or a search term is set. The design stage of this feature decides how
  it looks when a tag filter is set.
- Active tags are listed separately above the list, so they are not counted in the n of #195's
  "Filter (n applied)". The "Filter" menu has no tag picker (Out of scope).
