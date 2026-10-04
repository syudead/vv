# Contract: URL for the tag filter

Parent Issue: #193.

The source of truth for the library list (`/`) URL is #195's
[list-url.md](../../013-library-search/contracts/list-url.md). This document
covers only the parameter this feature adds and how it is handled. The folder
screen URL does not change.

## 1. Parameters

| Name | Value | When omitted |
| --- | --- | --- |
| `tag` | A tag `id`, one per selected tag (`?tag=3&tag=8`). At most 16 | No tag filter |

- The URL carries the `id`, not the name. A renamed tag keeps the same filter
  and URL, and choosing a tag by a synonym yields the original tag's `id`, so one
  condition never has two URLs (requirement 7).
- The `id`s are written in ascending order. The URL does not depend on the order
  of selection, which keeps the
  [listSnapshot](../../../web/src/api/listSnapshot.ts) keys matching.
- Non-numeric values, duplicates, and values beyond the 16th are dropped without
  an error.
- `id`s listed in the list response's `missingTagIds`
  ([tags-api.md §5](tags-api.md#5-list-filter-and-select-all)) are removed from
  the URL by replacing the current history entry, not adding one. The screen
  also reports that the tag no longer exists and refetches the tag list (Edge
  Case `ほかの画面での並行した変更`). The list itself is shown as the server
  returned it, with the missing `id`s ignored.
- The list snapshot (`listSnapshot`) key includes the normalized `tag`
  sequence, so returning from `/?tag=1` does not reuse the snapshot of `/`.
- When the list is restored from a snapshot, no list request is made and no
  `missingTagIds` arrives. The screen therefore compares the URL's `tag` with the
  shared tag list it refetches on restore. An `id` missing from the tag list is
  reported and removed from the URL, as when `missingTagIds` arrives. The
  condition changes, so the snapshot is not used and the list is refetched.

## 2. Adding and removing a tag

| Action | URL change |
| --- | --- |
| Press a tag on a library list card | Add its `id` to the current URL's `tag`; keep the search term (`q`), watch state, playability and sort (requirement 5). Nothing changes when the `id` is already in `tag`. With 16 tags already present, the tag is not added and the screen says it cannot be added |
| Press a tag on the video page, or choose a tag on the management page | Open `/?tag=<id>` with no other conditions (acceptance criteria 7 and 16). The sort is the one saved on the device, per #195's rules |
| Remove an active tag filter above the list | Remove only that `id` from `tag`; keep the other conditions |

- Each add or remove pushes one history entry, as #195's watch state does.
- #195's `条件を解除` also clears `tag`. #195's `ui-design.md` shows
  `条件を解除` only for watch state, playability and the search term. How it
  appears when a tag filter is active is decided in this feature's design
  stage.
- Active tag filters are listed separately above the list, so they do not count
  toward n in #195's `絞り込み（n 件適用中）`. The `絞り込み` menu has no tag
  selector (out of scope).
