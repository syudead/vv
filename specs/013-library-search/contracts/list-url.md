# Contract: URL for list conditions

Parent Issue: #195.

The library (`/`), the top of the folder screen (`/folders`) and each folder
(`/folders/{rootId}/{segment}…`) express the list conditions with the same query
parameters (requirements 16 and 18). The folder position is expressed by the
existing path. The visual layout and the controls are in the
[UI design](../ui-design.md).

## Parameters

| Name | Value | When omitted |
| --- | --- | --- |
| `q` | The query. Leading and trailing spaces are dropped, and it is cut at 100 code points (the same counting as the server's `[]rune`) | No search |
| `watch` | `unwatched` \| `inProgress` \| `watched` | All |
| `playable` | `1` | No filtering |
| `sort` | A value from [list-api.md, `VideoSort` values](list-api.md#videosort-values) | The sort saved on the device (default `addedDesc`) |
| `seed` | An integer from 1 to 2147483647 | For `sort=random`, the screen generates and adds one |

- `watch=all` and a false `playable` are not written into the URL, so that one
  set of conditions never has two URLs and the keys of
  [listSnapshot](../../../web/src/api/listSnapshot.ts) stay equal. (The key is
  built from the interpreted conditions, not the URL, so whether `sort` is
  present does not make the snapshot miss.)
- `sort` is always written when a condition change adds a history entry.
  Omitting it means "the sort saved on the device", so after changing the sort,
  Back could not return to the previous sort. `/` is opened without `sort` before
  any condition change, and right before the first history entry is added, the
  current entry is replaced with a URL that carries the current sort (Back then
  returns to a URL with the sort written, such as `/?sort=addedDesc`).
- Values that cannot be interpreted (unknown `sort` or `watch`, a non-numeric
  `seed`, `playable` other than `1`) are treated as the defaults without an error.
  A URL in the current form (`?q=…&sort=addedDesc`) gives a list with the same
  meaning (Edge Case "old URLs").
- When `sort=random` has no `seed` or a broken one, the screen generates a new
  `seed` and adds it to the URL without adding a history entry. From then on,
  reload, sharing and Back/Forward give the same order.
- "Shuffle" generates a new `seed` and changes the URL.

## Meaning on the folder screen

| Location and conditions | Behaviour |
| --- | --- |
| A folder, `q` not empty | Searches the folder and everything below it (`listFolderVideos` with `scope=subtree`) |
| The top (`/folders`), `q` not empty | Searches everything with `listVideos` |
| A folder, `q` empty, only `watch` or `playable` | Filters only the videos directly in the folder (`scope=direct`); the child folder cards are shown unchanged (requirement 20) |
| The top (`/folders`), `q` empty | Shows only the registered folder cards; `watch` and `playable` have no effect even if they stay in the URL. `ui-design.md` decides whether those controls are shown or hidden |

"Clear filters" removes `q`, `watch` and `playable`, and keeps `sort`, `seed`
and the folder position (requirement 21).

## History

- Watch state, playability, sort, direction, Shuffle and Clear filters each add
  one history entry (Back/Forward in acceptance criterion 12). The current screen
  rewrites the sort and the query with `replace`, so Back cannot return to the
  previous conditions. This changes to meet acceptance criterion 12.
- Typing a query adds only one history entry for one continuous input, from when
  focus enters the search field until it leaves or Esc exits. Each typed
  character does not add one.
- Only the sort (`sort`) is saved on the device; the query, the filters and
  `seed` are not. On a device that saved random, when the URL has no `sort`, the
  list opens with a new `seed`.
