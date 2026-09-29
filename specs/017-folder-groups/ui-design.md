# UI Design: List a folder group as one item and play it continuously

**Feature**: [parent Issue #326](https://github.com/syudead/vv/issues/326)

This design follows existing documents and does not decide their subjects again:

| Subject | Source |
| --- | --- |
| Visual rules, shell, list density | [Library UI: visual rules and list structure](../../docs/design-docs/library-ui.md) and `@theme` in [`web/src/index.css`](../../web/src/index.css) |
| Video page structure and the ended layer | [012 ui-design.md](../012-video-detail-ia/ui-design.md) |
| Folder page structure | [011 ui-design.md](../011-folder-browser/ui-design.md) and [013 ui-design.md "Folder screen"](../013-library-search/ui-design.md#folder-screen) |
| Tag chips, selection bar, combobox | [014 ui-design.md](../014-video-tags/ui-design.md) |
| Guest degradation | [016 ui-design.md "Guest degradation"](../016-single-account-auth/ui-design.md#guest-degradation) |
| Responses and routes the UI uses | [contracts/library-api.md](contracts/library-api.md) and [contracts/folder-groups-api.md](contracts/folder-groups-api.md) |
| Group watch status and the member to open | [data-model.md §6](data-model.md#6-group-watch-status-and-the-member-to-open) |

This document defines only what the following **add to or change in** existing screens: the group
card, the folder-derived tag distinction, the group order and autoplay notice on the video page,
and the grouping actions on the folder page and the video page. It adds no color, radius or shadow
token, and adds nothing to `pairs` in `tokens.test.ts` (see "Accessibility" below).

## Screen boundary

- **Library (`/`) grid and list view**: items become `LibraryItem`. A group item is shown as a group
  card (`web/src/library/`) or a group row. Ordinary video items keep the current `VideoCard` and
  `VideoRow` unchanged. No sections, headings or tabs are added (Requirement 15).
- **Tags on cards and on the video page**: folder-derived tags differ in shape from tags attached by
  hand (Requirement 13). The active tag filter row, the suggestion list and the management page do
  not change.
- **Selection bar**: the N in "N videos selected" and the "Remove tag" suggestions change
  (Requirements 21, 13). The component layout does not change.
- **Video page (`/videos/:id`)**: only when a group member is open, it adds one line above the title
  ("Group line"), the member order and a divider above the related videos column ("Member list"),
  and an autoplay notice at playback end ("Autoplay notice"). The page of a video outside any group
  is unchanged (second half of Requirement 27, Acceptance criterion 17).
- **Folder page (`/folders/{rootId}/…`)**: a grouping menu is added at the right end of the "Videos
  N" heading row ("Folder grouping menu"). The list still shows one video at a time (Requirement
  22). The top level (`/folders`) and search results get no menu.
- **Guest**: the group card omits the watch status and the watched count. The grouping menu, the
  video page menu and tags are not shown ([data-model.md §7](data-model.md#7-visibility-per-viewer)).

## Group card

### Structure

The card uses the same box as an ordinary video card (`VideoCard`: `rounded-lg`, `bg-surface`,
`shadow-card`, hover lift, selection `ring-2 ring-accent`) and sits in the same place at the same
size (Requirement 16, UI quality "Visual hierarchy", "Information density"). The thumbnail frame
draws the same folder artwork as the folder card on the folder page, so a group reads as "a folder
of related videos".

- **Folder artwork**: reuse `FolderArt` (`web/src/videoList/FolderArt.tsx`), shared with the folder
  card, as is. It stacks `previews` (up to 4 members with a generated thumbnail, in order) at an
  angle over a tabbed back panel. Moving the mouse sideways brings the image at that position to
  the front and plays its preview video. With no member thumbnail, only the back panel is drawn.
- **Count and length badge**: at the bottom right of the back panel (`right-5 bottom-4`), overlay
  "12 videos" then the total length, in the same badge as the video card's length (`bg-navbar/85`,
  `text-[11px]`, `tabular-nums`).
  - It sits above the image in front (`z-20`) and is not clickable (`pointer-events-none`).
  - Without a total length (`durationMs`), it shows only the count.
  - The watched count is not shown as a number. The artwork already shows a folder, so there is no
    lucide `Folder` marker either.
- **Top left**: the selection checkbox (same position as today).
- **Bottom progress**: only when `watchState = inProgress`, show the same `h-[5px]` bar as the video
  card, with the `bg-accent` width at `watchedCount / videoCount`.
  - For in progress with `watchedCount = 0` (the first member partly watched), only the bar's
    background (`bg-fg-subtle/50`) is visible, and it marks "started".
  - `aria-valuenow` of `role="progressbar"` uses the same ratio. The accessible name is "Share of
    videos watched".
  - Not shown for `unwatched` or `watched`.
- **Title**: the group name as `h3`, `text-sm font-medium`, truncated at 2 lines, full text in
  `title`. The same format as a video title (UI quality "Typography"). When `watched`, it uses
  `text-fg-muted` like a video.
- **Tag row**: the union of the members' tags (`tags`) in the same `CardTagRow` as the video card.
  A tag that is only folder-derived takes the "Folder-derived tag chip" shape. With no tags, the
  row is not shown.
- **Public badge**: a group item has no `public`, so it is not shown.
- Added time, size and the word "Folder" are not shown. The card adds only the count and length
  badge, so its height matches the video card (UI quality "Information density").

### Pressing and selection

- The whole card is one link to `/videos/{openVideoId}`, with `state.from` set to the current list
  URL (same as the video card). No member list page opens (Requirement 23, "Changes that do not
  satisfy the requirement").
  - Accessible name: "{name}, group of {N} videos". When `watchedCount` is 1 or more, append ",
    {M} watched". A guest gets the count only.
- Checking the selection checkbox (accessible name 'Select the group "{name}"') adds every member in
  `videoIds` to the selection. Unchecking removes every member. The card looks selected when every
  member is in the selection (Requirement 21).
- The N in "N videos selected" on the selection bar is the number of videos (members count), not
  the number of cards. Tag attach/detach and "Public" act on that number of videos.
- So "Select all" on the selection bar is not disabled by comparing the selected count with `total`
  (the number of items, which is cards), as today's `count >= total` does. Selecting one group
  would push the count above `total` and disable the button while unselected items remain.
  - It is disabled only while the request is in flight, and while the selection equals the set of
    `ids` in the response of the last "Select all" (`GET /api/library/ids`).
  - Removing or adding one video, or a condition change that clears the selection, enables it
    again.
  - When every loaded item is selected, it stays enabled, because unloaded items may remain.
- While in selection mode (one or more selected), pressing a card toggles its selection, like the
  video card.
- The card carries no ungroup or tag action (UI quality "Action priority"). Those are in "Group
  line" on the video page and in "Folder grouping menu" on the folder page.

### List view row

In the list view (library only), a group row fills the same columns as a video row (`VideoRow`):

| Column | Group row |
| --- | --- |
| Selection | The same checkbox as a video row, with the meaning in "Pressing and selection" |
| Thumbnail | The first member's thumbnail (first entry of `previews`). Bottom progress at `watchedCount / videoCount` when `inProgress` |
| Title | Group name (link to `/videos/{openVideoId}`). Below it, one `text-xs text-fg-muted` line with lucide `Folder` (`size-3`) and "12 videos" |
| Watched | `watched`: the same `Check` as a video. `inProgress`: "3 / 12" in `tabular-nums`. `unwatched`: empty |
| Length | Total length (empty when absent) |
| Quality | Empty (members differ, so it is not shown) |
| Size | Total |
| Added | `addedAt` (the latest member) |

### Refresh and removal

- A group card still in the list is refetched from `GET /api/folders/{rootId}/group` when a
  member's playback position, tags or `video` event changes. During the refetch the card does not
  change (no skeleton). The response replaces it on arrival.
- When the refetch returns 404, the card is removed from the list. The `total` in the count line
  may stay until the list is next loaded. Removing the card shows no message. The main case is
  returning from playback after an override change made the group single videos again, and
  reloading the list resolves it.

## Folder-derived tag chip

This adds a source distinction to the three tag chip shapes (014 "Tag chip"). Sizes (`h-5`, `h-6`,
`text-xs`) stay; shape and surface carry the difference (Requirement 13, UI quality "Typography").

| Source (`VideoTag`) | Shape |
| --- | --- |
| `manual` only, or both `manual` and `fromFolder` | The current video tag chip (`bg-elevated`, with a surface) |
| `fromFolder` only | No surface; a dashed border `border border-dashed border-border-strong`. Before the name, lucide `Folder` (`size-3`, `text-fg-subtle`, `aria-hidden`) |

- The dashed border is a shape meaning "borrowed from the folder", not a color-only difference (UI
  quality "Accessibility"). Text color matches the current chip (`text-fg-muted` on a card,
  `text-fg` on the video page). With no surface, the text sits on the parent surface (card
  `surface`, video page `bg`, the "+N" popover `elevated`). Every pair is already in `pairs`.
- Accessible name: in the clickable form (card), it stays "Filter by {name}", followed by a
  visually hidden "(from the folder name)".
- Pressing behaves the same regardless of source (filter by that tag).
- **Video page**: a `fromFolder`-only chip has no × (Requirement 13). The name part is the same
  link to `/?tag=<id>` as today.
  - A chip from both sources keeps the current shape (name + divider + ×). Pressing × removes only
    the tag attached by hand. After the response, the chip turns dashed in place (it does not
    disappear; Acceptance criterion 9).
  - Focus moves to the × of the next chip, else of the previous chip, else to the "Add tag" input
    (the 014 rule; dashed chips have no × and are skipped).
- **"Add tag" suggestions**: a tag attached to the video only from a folder is still suggested.
  Confirming it attaches it by hand too, and the chip turns into the shape with a surface.
- **"Remove tag" on the selection bar**: suggestions are only summary tags with `manualCount >= 1`.
  - Folder-derived-only tags cannot be removed, so they are not suggested (no `disabled` row
    either).
  - The "some" check and the count use `manualCount`. The denominator in "Some: 1 / 3" stays the
    selected count.
  - When no selected video has a tag attached by hand, the message is "The selected videos have no
    tags that can be removed". It does not say "There are no tags", because folder-derived tags may
    exist.
- Order stays the API order (natural name order), not split by source.

## Video page

### Group line

Only when `Video.group` exists, one line goes **above** the title (`h1`) (Requirement 25). It is
the first child of the title and tags block (`flex flex-col gap-2`), with `gap-2` to the title.

- Content, left to right: lucide `Folder` (`size-3.5`, `text-fg-subtle`, `aria-hidden`) → group
  name (`text-fg-muted`, one line truncated, full text in `title`) → "·" in `text-fg-subtle` →
  "3 / 12" (`tabular-nums`). The whole line is `text-xs` (`text-sm` from `sm`). It is smaller than
  the title (`text-xl` to `text-2xl`) and in a secondary color, so it does not draw the eye before
  the title despite sitting above it (UI quality "Visual hierarchy").
- The group name is not a link, because the breadcrumb in the header already links to the same
  folder.
- For the **owner**, the whole line is a ghost `sm` `Button` (`-ml-2` aligns the text's left edge
  with the title), ending in `ChevronDown` (`size-3.5`), and opens `ui/Menu` (Requirement 29). The
  accessible name is 'Group "{name}", video 3 of 12. Grouping menu'. Items:
  - "Ungroup" (lucide `Ungroup`): `PUT /api/folders/{rootId}/grouping`, `mode: ungroup`.
  - "Turn the group into a tag" (lucide `Tag`): `POST /api/folders/{rootId}/grouping/tag`. Not
    shown when `group.folder.path` is empty (the media folder itself; contracts/folder-groups-api.md
    §2).
  - No "Group this folder's videos". The video page already shows a group, so it would mean
    nothing.
- While the menu is open, Esc only closes the menu and does not close the page. Focus returns to the
  trigger (the `ui/Menu` default). The video page receives Esc in the `window` capture phase. Like
  the current exceptions (speed menu, popovers), an open `ui/Menu` (`role="menu"`) is added to the
  exclusions. This adds one exception to the Esc exceptions in 012 "Interaction details".
- For a **guest**, it is a non-clickable text line (`Video.group` is built from public members for
  a guest too).
- After an action:
  - Refetch the video (`GET /api/videos/{id}`) and the related videos. `group` is gone, so this
    line, "Member list" and the neighbor arrows return to the ordinary video shape.
  - Discard the library cache, so the list reloads on close (Structural Decision 10, Acceptance
    criteria 3, 4, 5).
  - Report with a toast:
    - Ungroup: 'Ungrouped "{name}"'
    - Turned into a tag: 'Created the tag "{name}" and ungrouped' when `created` is true, otherwise
      'Added the tag "{name}" and ungrouped'
- While the request is in flight, the menu trigger is `disabled`. The action is reached by opening
  a menu, so no confirmation dialog is shown for undo. Both actions can be reverted to "Automatic"
  from the folder page menu (the tag can be deleted on the management page).
- Failure: report with a toast; the page does not change.
  - 400 `invalid_request` (tag name rules): '"{name}" can't be used as a tag name, so it can't become
    a tag' (Edge case "Names that cannot become a tag")
  - 409 `conflict`: "This folder is no longer a group", then refetch the video and the related
    videos (another tab changed it first)
  - 404 and others: "Couldn't make the change"

### Member list

This is the right column (below the player under `lg`) when `RelatedVideos.group` exists
(Requirement 26).

- The column's first heading (`h2`, `text-sm font-semibold text-fg`) becomes "Up next", with "3 /
  12" in `text-xs text-fg-muted tabular-nums` to its right (`flex items-baseline justify-between`).
  The reader then learns that the list at the top is the autoplay range before seeing the divider.
- Below it, every member is listed in order (not truncated, even for hundreds; Edge case "Large
  group"). Rows keep the density of the current related video rows (thumbnail `w-40`, length label,
  title `text-sm font-medium`, row gap `gap-3`, hover `bg-hover-wash`; UI quality "Information
  density"), with these additions:
  - The order number at the start of the row (left of the thumbnail) in `w-5 text-right text-xs
    text-fg-muted tabular-nums`. The number shows the current position and what remains without
    counting.
  - Watched member: the title in `text-fg-muted`, with lucide `Check` (`size-3.5`, `text-success`)
    to its right. Screen readers get a visually hidden "Watched"; the icon is `aria-hidden`.
  - Partly watched member: the same bottom progress bar as today.
  - **Current member**: not a link but a row with `aria-current="true"`, surface `bg-active-wash`,
    and `border-l-2 border-accent` on the left (the left edge of the row's `-m-1.5 p-1.5` outline).
    A visually hidden "Now playing" precedes the title. It is not clickable, so its hover surface
    does not change either.
- **Divider**: one `border-t border-border` line between the member order and the related videos.
  - Spacing: `pt-5` above and `pb-2` below (one step larger than the row gap `gap-3`; UI quality
    "Spacing rhythm").
  - Below the line, the `h2` "Related videos" (the current heading), then the current related video
    list (without members of the same group; contracts/folder-groups-api.md §3).
  - With 0 related videos, neither the divider nor the "Related videos" heading is shown.
- Scrolling at `lg` and above: as in 012, the column's first heading row (here "Up next") stays at
  the top. Everything below it (member order → divider → "Related videos" → related videos) scrolls
  inside one container.
  - On opening the page (including moving to another member), `scrollIntoView({ block: "nearest"
    })` moves only the container so the current member's row is visible in it (Edge case "Large
    group", Acceptance criterion "hundreds of videos"). The page and the left column do not move.
  - Under `lg`, the page does not move (the player would leave the top of the screen).
- Group member rows play the list preview on hover, like related video rows.
- Loading and failure look like the current related videos (6 skeleton rows, "Couldn't load related
  videos" and "Retry"). Without the related videos response the member order cannot be shown
  either, so the heading stays "Related videos" and changes to "Up next" when the response arrives.
- **Guest**: `group.items` holds public members only, with no watched marker and no progress (no
  `progress`). Numbers are the order among public members.

### Neighbor arrows

The neighbor arrows (012 "Neighbor arrows") follow `prevId` and `nextId` in the related videos
response. For a member they become previous and next within the group; the first member has no left
arrow and the last member has no right arrow (Requirement 28). Accessible names and tooltips stay
the current "Previous video: {title}" and "Next video: {title}". Titles come from `group.items`.

### Autoplay notice

When a member finishes playing and `nextId` exists, a notice layer replaces the current ended layer
(Requirement 27). Stacking order, `bg-overlay` and keeping the control bar visible are the same as
012 "Ended", and only one layer shows at a time.

- A centered panel (`bg-navbar`, `rounded-lg`, `p-5`, `max-w-lg`) stacks these vertically
  (`gap-3`):
  - Line 1: "Up next" in `text-xs font-semibold text-accent`, with "in 5 seconds" (the remaining
    seconds, counting down every second) in `text-xs text-fg-muted tabular-nums` to its right.
  - A link to the next member: thumbnail (`w-56`, hidden under `sm`) and title (`text-base
    font-semibold`, truncated at 2 lines). The same shape as the current "Next video".
  - A full-width bar `h-1 rounded-full bg-fg-subtle/50` in which the `bg-accent` remainder shrinks
    from the right over 5 seconds. With reduced motion, the bar stays but the smooth shrink stops,
    and it shrinks in steps as the seconds tick (`motion-reduce:transition-none`; the state stays
    visible and only the motion stops, library-ui.md §4, 012 "Interaction details").
  - Actions: `secondary` "Cancel" (lucide `X`) on the **left**, `primary` "Play now" (`Play`) to its
    right. DOM order matches, so Tab reaches "Cancel" first (UI quality "Action priority").
- The wait is 5 seconds, counted from when the notice appears. At 0 the page moves to the next
  member (keeping `state.from` and starting playback there).
- **Cancel**: removes the notice layer and shows the current ended layer ("Next video" = the next
  member, "Play next", "Watch again") (Requirement 27). If focus was on "Cancel", it moves to "Play
  next".
- **Esc**: while the notice layer shows, Esc cancels and does not close the page. After the notice
  is canceled, Esc closes the page as today. This adds one exception to the Esc exceptions (speed
  menu, fullscreen) in 012 "Interaction details".
- **The next member is gone** (404 on the check in Structural Decision 12): remove the notice layer
  and show the ended layer for "no next video" ("Watch again" only). No message states the reason.
  The next open decides with the latest group.
- **Last member** (no `nextId`): no notice; the current layer with only "Watch again" appears
  (Acceptance criterion 16).
- **An auto-opened member fails to play**: it stops at the current playback failure layer
  (`PlaybackFailure`) and goes no further (Edge case).
- Pressing "Restart" or "Play" on the control bar removes the notice and resumes playback, as with
  the current ended layer.
- **Screen readers**: when the layer appears, a visually hidden `role="status"` reads 'Playback
  finished. The next video, "{title}", plays in 5 seconds' **once**. The remaining-seconds text and
  the bar are `aria-hidden`, so they are not reread every second (UI quality "Accessibility").
  Focus moves to "Cancel" only when focus was inside the player (same as 012 "Ended").
- Touch center controls are not shown while the notice layer shows (012 "Overlay layer").

## Folder grouping menu

The folder's grouping actions go in the "Videos N" heading row of the folder page (Requirement 22,
Requirements 7, 11). Subfolder cards get none. A card is a link for opening, and adding actions to
it would double the Tab stops (the same reason as 013 "Search results").

- The `Section` heading row becomes `flex items-center justify-between`: the current `h2` "Videos
  N" on the left, and a ghost `sm` `Button` trigger on the right. The trigger holds a lucide icon +
  label + `ChevronDown` (`size-3.5`), which vary with `FolderSummary.grouping.grouped`.

  | `grouped` | Icon | Label |
  | --- | --- | --- |
  | true | `Group` | "1 item in the library" |
  | false | `Ungroup` | "One by one in the library" |

  Accessible names: "Grouping in the library: shown as 1 item. Open the menu" and "…: shown one by
  one. …". The label states the current state, and pressing changes it. `grouped` results from the
  override and the automatic rule. So with an override but only one direct video, the label stays
  "One by one", which shows here that the override has no effect.
- The trigger is `text-fg-muted` (the ghost default), as weak as the heading (`text-xs
  font-semibold text-fg-muted`). It does not draw the eye before the grid (UI quality "Action
  priority").
- Menu contents (`ui/Menu`, `align="end"`), top to bottom:
  - `MenuLabel` "Grouping in the library"
  - `MenuRadioGroup` (the current `grouping.mode`): "Automatic" (`auto`), "Don't group"
    (`ungroup`), "Group this folder's videos" (`groupDirect`). No per-item descriptions; the line
    below explains.
  - A `text-xs text-fg-muted` line "Automatic: groups folders below a media folder that have no
    subfolders and 2 or more videos" (same padding as `MenuLabel`, not clickable). It states all
    three conditions of Requirement 2, so the reader can tell that "Automatic" does not group on the
    media folder's own page either.
  - Only when `taggable` is true: a separator and "Turn the group into a tag" (lucide `Tag`). When
    false, the item is not shown (contracts/folder-groups-api.md §2). Rejected: keeping a disabled
    item, because for the media folder itself the reason does not fit in one line.
- Choosing a radio sends `PUT /api/folders/{rootId}/grouping` and updates the trigger label and the
  radio from `grouping` in the response. No toast; the changed label is the result. Choosing the
  same value again also sends the request (200).
- "Turn the group into a tag" sends `POST …/grouping/tag` and reports with a toast (the same text as
  "Group line"). The trigger updates from `grouping` (`ungroup`) in the response.
- Both discard the library cache on response (Structural Decision 10). The next time the library
  opens, its cards change without a rescan (Acceptance criteria 3, 4, 5).
- A radio change does not reload the folder page list, which stays one video at a time with
  unchanged contents (only `grouping` of `FolderSummary` is replaced from the response).
- A successful tag conversion replaces `grouping` and also reloads the folder page video list
  (keeping the scroll position). A new tag attaches as a folder-derived tag to the videos inside
  from the next read (data-model.md §4), but `tags` on the shown cards is not in the response and
  is stale. The reload shows the result of Requirement 11 in place.
- While the request is in flight, the trigger is `disabled` and its icon becomes `LoaderCircle`
  (`animate-spin`).
- Failures are toasts:

  | Status | Toast |
  | --- | --- |
  | 400 `invalid_request` | '"{name}" can't be used as a tag name, so it can't become a tag' |
  | 409 | "This folder is no longer a group" (refetch the folder list) |
  | 404 | "The folder wasn't found." |
  | Others | "Couldn't make the change" |

- A folder without a "Videos" section (no direct videos) has no heading row, so it has no menu. The
  media folder itself (empty `path`) shows the same menu in the same row ("Group this folder's
  videos" works on a root too; Requirement 7).
- A **guest** has no `grouping`, so the trigger is not shown (the heading row stays as today).
- Subfolder cards get no group marker. The requirements do not ask for one, and more information in
  the lower half of the card would compete with the "count" line from 011.

## Interaction states

- Group card and row: the same as `VideoCard` and `VideoRow` (hover lift and shadow,
  `has-[a:focus-visible]` outer outline, selection `ring-2 ring-accent`, `select-none` in
  `selectionMode`).
- Dashed chip: on hover, `text-fg`, and the border turns solid while staying
  `border-border-strong` (a `ring` on a shape without a surface would look like a double border).
  Focus uses the global `:focus-visible` (inside a card, the same inner outline as 014).
- "Group line" trigger and "Folder grouping menu" trigger: inherit the ghost `Button` states.
  `disabled` only while sending.
- Member rows: the same hover as related video rows. The current member row is not clickable and
  does not change on hover.
- Notice layer actions: inherit the secondary and primary `Button` states.

## Accessibility

- Accessible names:
  - Group card link: "{name}, group of {N} videos" (for the owner with `watchedCount >= 1`, append
    ", {M} watched")
  - Group card checkbox: 'Select the group "{name}"'
  - Group card progress: "Share of videos watched"
  - Dashed chip: "Filter by {name}" + a hidden "(from the folder name)"
  - "Group line" trigger: 'Group "{name}", video {M} of {N}. Grouping menu'
  - Member row: "{title} {length}", as for related videos today. The current member has
    `aria-current="true"` and a hidden "Now playing"; a watched member has a hidden "Watched"
  - Notice layer: `role="status"` with 'Playback finished. The next video, "{title}", plays in 5
    seconds' (once). "Cancel" and "Play now"
  - "Folder grouping menu" trigger: "Grouping in the library: shown as 1 item. Open the menu" and
    "Grouping in the library: shown one by one. Open the menu"
- Grouping: the member order is a `ul` under the "Up next" `h2`, and the related videos are a `ul`
  under the "Related videos" `h2`. The two `ul` elements are not merged, so the divider is audible
  to screen readers too.
- The notice's remaining seconds and bar are `aria-hidden`. Cancel works both with the first button
  Tab reaches and with Esc.
- Color pairs: no new pair is used. Dashed chip text sits on the parent surface (`surface`, `bg`,
  `elevated`) in `fg` or `fg-muted`. Notice layer text sits on `navbar` in `fg`, `fg-muted` or
  `accent`. All are in `pairs`. `border-border-strong`, `bg-active-wash` and `bg-fg-subtle/50` are
  not text and are translucent, so `pairs` does not cover them. No new token is added.
