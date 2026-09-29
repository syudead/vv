# Contract: URL that encodes the list conditions

Parent Issue: #195.

The library (`/`), the top level of the folder page (`/folders`) and each folder
(`/folders/{rootId}/{segment}…`) encode the list conditions with the same query parameters
(Requirements 16 and 18). The existing path encodes the folder position. The layout and controls
are in the [UI design](../ui-design.md).

## 1. Parameters

| Name | Value | When omitted |
| --- | --- | --- |
| `q` | Search term. Leading and trailing whitespace is trimmed, and the term is cut at 100 code points (the same count as the server's `[]rune`) | No search |
| `watch` | `unwatched` \| `inProgress` \| `watched` | All |
| `playable` | `1` | No filtering |
| `sort` | A value from [list-api.md §3](list-api.md#3-videosort-values) | The sort saved on the device (default `addedDesc`) |
| `seed` | Integer from 1 to 2147483647 inclusive | For `sort=random`, the UI generates one and adds it |

- `watch=all` and a false `playable` are not written to the URL. This keeps one URL per condition
  and keeps the keys of [listSnapshot](../../../web/src/api/listSnapshot.ts) matching. The key is
  built from the parsed conditions, not from the URL, so the presence of `sort` does not make the
  snapshot miss.
- `sort` is always written when a condition change adds a history entry.
  - Why: omitting it means "the sort saved on the device", so Back after a sort change could not
    return to the previous sort.
  - `/` opens without `sort` before any condition change. Right before the first history entry
    is added, the UI replaces the current entry with a URL that carries the current sort. Back
    then returns to a URL with the sort, such as `/?sort=addedDesc`.
- Values that cannot be parsed (unknown `sort` or `watch`, a non-numeric `seed`, a `playable`
  other than `1`) are treated as the defaults and raise no error. A URL in the current format
  (`?q=…&sort=addedDesc`) produces a list with the same meaning (Edge case "old URLs").
- For `sort=random` with a missing or broken `seed`, the UI generates a new `seed` and appends it
  to the URL without adding a history entry. From then on, reload, sharing and Back/Forward give
  the same order.
- "Shuffle" generates a new `seed` and changes the URL.

## 2. Meaning on the folder page

- When `q` is not empty, the search covers the folder and everything below it (`listFolderVideos`
  with `scope=subtree`). At the top level (`/folders`), `listVideos` searches everything.
- When `q` is empty and only `watch` or `playable` is set, only the videos directly in the folder
  are filtered (`scope=direct`). The subfolder cards stay as they are (Requirement 20).
- At the top level (`/folders`) with an empty `q`, only the registered folder cards are shown.
  `watch` and `playable` have no effect even when they remain in the URL. `ui-design.md` decides
  whether those controls are shown or hidden.
- "Clear filters" removes `q`, `watch` and `playable`, and keeps `sort`, `seed` and the folder
  position (Requirement 21).

## 3. History

- Watch status, playability, sort, direction, "Shuffle" and "Clear filters" each add one history
  entry (Back/Forward in Acceptance criterion 12). The current UI rewrites the sort and the
  search term with `replace`, so Back cannot return to the previous conditions. This changes to
  satisfy Acceptance criterion 12.
- Typing a search term adds one history entry per continuous input: from when the search box
  gains focus until it loses focus or the user leaves it with Esc. Each typed character does not
  add an entry.
- The device saves only the sort (`sort`). It does not save the search term, the filters or
  `seed`. On a device that saved random order, a URL without `sort` opens with a new `seed`.
