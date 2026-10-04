# UI design: Telling tentative tags apart and settling them by confirm, reject or merge

**Feature**: [parent Issue #589](https://github.com/syudead/vv/issues/589) ·
[plan.md](plan.md) · [contracts/screen-api.md](contracts/screen-api.md) ·
[research.md R-5](research.md#r-5-reject-applies-only-to-tentative-tags-a-confirmed-tag-gets-409-tag_not_tentative-and-no-change) ·
[R-8](research.md#r-8-the-tentative-only-filter-and-the-rejected-name-list-live-in-the-screen)

Sources: the visual rules come from these documents and are not decided again
here.

| Topic | Source |
| --- | --- |
| Colour, interaction states, width breakpoints, list and video page layout | [Library UI](../../docs/design-docs/library-ui.md) (the cards in "6. List layout" and "8. Video page layout") |
| Role tokens | `@theme` in [`web/src/index.css`](../../web/src/index.css). Referenced by name; values are not copied |
| Pairs checked for contrast | [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts) |
| The three tag chip forms, the card tag row, video page tags, and the tag management page skeleton (rows, create and rename, the synonyms dialog, the merge and delete dialogs, states) | [specs/014-video-tags/ui-design.md](../014-video-tags/ui-design.md) and the current [`web/src/tags/`](../../web/src/tags/), [`web/src/library/CardTagRow.tsx`](../../web/src/library/CardTagRow.tsx), [`web/src/player/VideoTags.tsx`](../../web/src/player/VideoTags.tsx) |
| The chip of a folder-only tag (dashed border and Folder mark) | [specs/017-folder-groups/ui-design.md "Folder-derived tag chip"](../017-folder-groups/ui-design.md#folder-derived-tag-chip) |
| Where screen text lives and its formatting | [Screen text and formatting (i18n)](../../docs/design-docs/i18n.md). The English here states intent; after implementation the catalog [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) is the source of truth |

This feature adds four owner-only things to the screens:

1. A **tentative mark** on video tag chips (library cards, group cards, the video
   page) (requirement 6, acceptance criterion 5)
2. A **tentative mark** on tag management rows, and a **tentative-only** filter
   (requirements 6 and 7, acceptance criteria 5 and 6)
3. **Confirm** and **Reject** on tentative rows (merge stays the current
   action), replacing "Delete…" (requirements 8 and 10, acceptance criteria 7, 8
   and 12)
4. The **rejected names** list on the tag management page, with removal
   (requirement 14, acceptance criterion 13)

Nothing else changes. A tentative tag behaves like a confirmed tag in attaching,
detaching, filtering and search (requirement 4), so no mark is added to the
active-filter tag row, the candidates of "Add tag" (video page) and of the
selection bar's "Add tag" and "Remove tag", the merge-target candidates, the
selection bar, the list view, or the location row of folder-page search results.
Candidate rows get no mark because a candidate answers "which tag to attach", and
being tentative does not change that choice (the attached tag stays tentative;
[data-model.md §3](data-model.md#3-write-rules)). Guests see no tags
(requirement 4), so none of the four appears for them. No new colour, radius or
shadow token is added, and nothing is added to `pairs` in `tokens.test.ts` (see
"Colour" below).

## Why this shape

- **One tentative mark (lucide `CircleDashed`) on every screen.** A dashed circle
  is the shape current development and issue-tracking products have converged on
  for "draft, not yet decided", and it gives the state a shape distinct from 017's
  Folder mark, which means origin. The mark goes **after** the name, not before.
  Before the name it would sit beside the Folder mark and read as "two origins",
  and it would delay the start of the name. The colour is `text-fg-subtle`,
  lighter than the name (`text-fg-muted` or `text-fg`). A tentative tag is still a
  usable tag, so no warning colour, surface colour change, badge or count is used
  (`UI品質` "visual hierarchy" and "what does not satisfy the requirement").
- **"Tentative only" is a toggle button (`aria-pressed`) on the search row and is
  not stored in the URL.** It matches 014's tag management search, which filters
  in place without a URL, so the clean-up work (filter → settle rows one by one →
  the list empties) stays on one screen. While pressed, the result is the list
  shrinking, and the count row "3 of 12 tags" tells how many remain (avoiding the
  `UI品質` failure "finding tentative tags still requires reading every tag").
- **"Confirm" sits on the row; "Reject" sits in the menu.** Confirming only moves
  the state forward and costs nothing to undo, and it is the main clean-up action,
  so it goes first in the row's action group and takes one press. Rejecting
  deletes the tag, so it inherits the place 014 gave "Delete…" (below the divider
  in "More actions", danger) and goes through a confirmation dialog (`UI品質`
  "action priority"). Merge stays where it is as "Merge into another tag…".
- **Rejected names are a collapsible section below the list.** They are not tags,
  only a record of "do not create this again", and are not something to check on
  every step of the clean-up (`UI品質` "information density"). They are not on a
  settings page or a separate screen so that right after rejecting, the user can
  see on the same screen where the name went, and undoing it (removal) happens in
  place.

## Words

| Place | Text |
| --- | --- |
| Visually hidden text, `title` and tooltip of the tentative mark | Tentative |
| Accessible name of a card chip (pressable) | Filter by {name} (tentative) |
| Accessible name of a card chip (folder-only and tentative) | Filter by {name} (from the folder name, tentative) |
| Accessible name of the name part on the video page | Filter by {name} (tentative) |
| Filter button | Tentative only |
| Filter button hint (tooltip) | Show only tags created by automatic tagging |
| Count row (while filtering; same form as search) | 3 of 12 tags |
| Empty-state heading when there are no tentative tags | No tentative tags |
| Empty-state description when there are no tentative tags | Tags created by automatic tagging appear here until you confirm or reject them. |
| No search match while filtering | No tentative tags match "{query}" |
| Button in the empty and no-match states | Show all tags |
| Row "Confirm" (accessible name and tooltip) | Confirm |
| Menu "Reject" | Reject… |
| Reject dialog heading | Reject "{name}" |
| Reject dialog body (N videos) | This tag will be removed from N videos, and automatic tagging won't create "{name}" again. You can allow the name again from the rejected names below. |
| Reject dialog body (0 videos) | This tag isn't on any videos. Automatic tagging won't create "{name}" again. You can allow the name again from the rejected names below. |
| Reject dialog buttons | Cancel / Reject (Rejecting… while submitting) |
| Confirm toast | Confirmed "{name}" |
| Reject toast | Rejected "{name}" |
| `tag_not_tentative` toast | This tag was already confirmed, so the list was reloaded |
| Rejected names heading | Rejected names |
| Rejected names description | Automatic tagging won't create these tags. Remove a name to allow it again. |
| No rejected names | No rejected names |
| Accessible name of a name's × | Allow "{name}" again |
| Rejected names failed to load | Couldn't load the rejected names |
| Removal failed | Couldn't remove "{name}": {reason} |
| `tag_not_tentative` in `errorText` | The tag is already confirmed. |

- "Tentative" is used only as the mark's text, and "automatic tagging" only in
  descriptive sentences. "Draft" and "pending" are not used ("pending" is the
  term for an ingest stage).
- Tag names are user data and are embedded untranslated (i18n.md).

## Tentative mark

A tentative tag (`tentative: true`) has the same single mark on every screen.

- lucide `CircleDashed`, `shrink-0`, `text-fg-subtle`, `aria-hidden`, placed
  **after** the name with `gap-1`. Size `size-3` on chips (the same as the Folder
  mark) and `size-3.5` on tag management rows.
- When the name is truncated (`truncate`), the mark is not; it stays after the
  end of the truncated name (the mark is `shrink-0` and the name is
  `min-w-0 truncate`).
- For screen readers, a pressable element appends "(tentative)" to its accessible
  name; a non-pressable element places a visually hidden "Tentative" after the
  name. An element's `title` stays the name (it exists to show the full name), and
  the mark itself gets no tooltip. Only tag management rows give the mark a
  `Tooltip` "Tentative" (see "Row" below).
- Confirmed tags get nothing. Confirmed tags look exactly as before this feature
  (acceptance criteria 5 and 7).

## Video tag chips

### Library card and group card

Each chip in [`CardTagRow`](../../web/src/library/CardTagRow.tsx) (the visible
chips, the chips inside the "+N" popover, and the measurement-only row) gets the
mark above.

| Chip | Order |
| --- | --- |
| Chip with a surface (`bg-elevated`, `h-5`, `text-xs`, `text-fg-muted`) | Name → mark |
| Folder-only chip (dashed border) | Folder mark → name → tentative mark. The two marks have different shapes, so they do not blur together |

- The chip widens by the mark's width, and the number that fits in the row (the
  "+N" calculation) follows from the re-measured widths. The height does not
  change (`UI品質` "spacing rhythm and typography").
- Pressing the chip (filter by that tag, or toggle card selection while selecting)
  behaves the same whether the tag is tentative or not (requirement 4, acceptance
  criterion 4).
- The tag row on group cards is the same component, so it takes the same form.

### Video page

Chips in [`VideoTags`](../../web/src/player/VideoTags.tsx) (`h-6`, `text-xs`,
`text-fg`) get the mark.

| Chip | Order |
| --- | --- |
| Chip with a surface (name + divider + ×) | Name → mark, inside the name part (`Link`). The divider and × are unchanged. Pressing × removes the tag from this video, and the chip disappears when the response arrives (as today) |
| Folder-only chip (dashed, no ×) | Folder mark → name → tentative mark |

- The `tentative` in the attach-by-name response (`tag` of `POST /api/video-tags`)
  goes onto the chip as is. A missing name created through "Add tag" is created
  as a confirmed tag (requirement 16), so it shows no mark. If a tentative tag is
  among the "Add tag" candidates, choosing it adds a chip with the mark.
- After confirming or rejecting on the tag management page, chips on an open
  video page may stay stale until the video is next reloaded (the same as 014
  "Stale tags in other screens"). Pressing one that returns `tag_not_found`
  reloads, as today.

## Tag management page

### Toolbar

The action row (search and "New tag") gains a **tentative-only** toggle button.

- The component is a secondary `Button` (`h-9`, the same height as the search
  input) with lucide `CircleDashed`, the text "Tentative only", and
  `aria-pressed`. While pressed it looks like an `IconButton` in its `active`
  state (`border-accent-active`, `bg-accent-soft`, `text-link`), the same
  "selected" form as the toolbar's view-mode and sort selections. No custom state
  colour is introduced.
- At `sm` and up, from the left: the search input (`flex-1 sm:max-w-sm`) →
  "Tentative only" → "New tag" (primary) at the right end. Below `sm`, the search
  input is on the first row, and "Tentative only" and "New tag" share the second
  row in `grid grid-cols-2 gap-2` (both full width). The first Tab stop in the
  body is still the search input, and the `/` key is unchanged.
- Pressing it limits the list to tags with `tentative` true. It combines with
  search (both filter). The count row takes the same "3 of 12 tags" form as
  search (the denominator is all tags). The filter is state of this screen and is
  not stored in the URL or `localStorage` (see "Why this shape"). It is cleared on
  leaving the screen.
- While not pressed, when there are no tags at all (the "No tags yet" state), it
  is `disabled`, like the search input. It is also `disabled` while loading and
  after a load failure. **While pressed, it is not disabled even when no tags
  remain** (after rejecting the last tag, or when a reload finds none), because it
  is the focus destination and the only way to clear the filter. The list then
  shows the "No tentative tags" empty state in "States" below. If releasing it
  leaves zero tags, the button becomes `disabled`, so focus moves to "New tag"
  (also when released through "Show all tags").
- A row being renamed stays in the list even when it stops matching the filter,
  as with search (renaming confirms the tag and moves it out of the filter; it
  leaves when the rename response arrives and the editor closes).
- **Focus when a row leaves the filter**: while "Tentative only" is pressed, when
  confirm, rename, adding a synonym or merge turns a row confirmed and removes it
  from the list, the operation's current focus target (the same row's "Rename" or
  "Synonyms", or the merge target's name) no longer exists. Focus then follows
  the delete rule: "Rename" on the next row at the removed row's position,
  otherwise the previous row, and if no row remains, "Tentative only" (the filter
  button rather than "New tag", because the next action is to clear the filter).
  When a reject or a `tag_not_found` reload removes a row while pressed, the same
  rule applies with "Tentative only" as the last destination. When the target row
  is still in the list, each operation keeps its current focus target.
  **Adding a synonym is the exception, because it happens inside a dialog: the
  rule applies when the dialog closes.** If adding removes the row from the list,
  the dialog stays open (014 lets the user add the next name), and focus stays in
  the dialog's input (the background is `inert` through `ModalFrame`, so focus
  cannot move there). When the dialog closes (× or Esc), focus goes to the row's
  "Synonyms" if the row is still listed, as today; otherwise it follows this rule
  from the row's position when the dialog opened: next row's "Rename" → previous
  row → "Tentative only" (today's `cancelSynonyms` points at the vanished row and
  falls back to "New tag"; this changes that). The implementation's tests cover:
  merging one tentative tag into another while pressed (focus goes to "Tentative
  only" or an adjacent row, never to "New tag"); rejecting the only tag ("Tentative
  only" stays pressable and holds focus); and adding a synonym to the only
  tentative tag while pressed, then closing the dialog (focus stays in the input
  after adding, and moves to "Tentative only" on close).

### Row

On a tentative row only the name column and the action group change. The row
height (`py-2`), the count column and the rename input are unchanged (`UI品質`
"information density").

- **Name column**: the tentative mark (`size-3.5`) after the name `Link` with
  `gap-1`. The mark gets a `Tooltip` "Tentative" and a visually hidden
  "Tentative". The name and synonym-line formatting is unchanged. A tentative tag
  has no synonyms (requirement 9), so a tentative row has no synonym line. While
  renaming, the input replaces the name, so the mark is not shown.
- **Action group** (from the left): "Confirm" (ghost `IconButton`, `sm`, lucide
  `Check`) → "Rename" → "Synonyms" → "More actions". "Confirm" appears only on
  tentative rows (confirmed rows keep their current three actions). It comes
  first so the main clean-up action is the first one reached in the row, and its
  colour matches the other `IconButton`s (no accent colour or surface; `UI品質`
  "visual hierarchy").
- **"More actions" menu** (tentative row): "Merge into another tag…" (`Merge`), a
  divider, "Reject…" (lucide `Ban`, `tone="danger"`). "Delete…" is not shown
  (requirement 10, acceptance criterion 12). The menu on confirmed rows is
  unchanged ("Merge into another tag…", divider, "Delete…").
- When the tag is renamed, gains a synonym, or becomes a merge target, the
  response's `Tag` comes back with `tentative: false`
  ([contracts/screen-api.md §1](contracts/screen-api.md#1-changed-existing-routes)).
  The row is replaced with it: the mark and "Confirm" disappear and the menu
  takes the confirmed-row form (acceptance criterion 11). A rename to the current
  name changes nothing and the tag stays tentative.
- The "Synonyms" dialog (014 "Synonyms") opens the same way for tentative tags.
  The chip list at its top is empty, and adding a name with "Add synonym"
  confirms the tag. The dialog's text does not change. Focus when adding removes
  the row from the list while "Tentative only" is pressed follows "Focus when a
  row leaves the filter" in "Toolbar" above (applied when the dialog closes).

### Confirm

- Pressing it sends `POST /api/tags/{id}/confirm` immediately. There is no
  confirmation dialog (undoing has no cost; no requirement asks to make a tag
  tentative again, and confirming does not change whether the tag is usable, so
  there is nothing to undo).
- While submitting, the button's icon becomes `LoaderCircle`
  (`animate-spin motion-reduce:animate-none`) with `aria-busy` and
  `aria-disabled="true"`, and **further presses (double clicks, repeated keys)
  are ignored so only one request is sent**. It is not `disabled` because focus is
  on the pressed button and `disabled` would drop focus to `body` (on failure,
  focus stays on this button). That row's "Rename" and "More actions" are
  `disabled` (the same as `blockStart` while a rename submits; other rows are
  unaffected).
- On `200`, the row is replaced with the response's `Tag` (the mark and "Confirm"
  disappear; acceptance criterion 7). Toast "Confirmed "{name}"" (like the
  delete and merge toasts). Focus moves to the same row's "Rename" in place of
  the vanished "Confirm". While "Tentative only" is pressed, the row leaves the
  list, so focus follows "Focus when a row leaves the filter" above: the next
  row's "Rename", otherwise the previous row, and if none remains, "Tentative
  only" (the filter button rather than "New tag", because the next action is to
  clear the filter).
- An already confirmed tag (confirmed first in another tab; the response is still
  `200`) is handled the same way.
- A failure with `tag_not_found` is handled like today's "the tag no longer
  exists": toast "This tag no longer exists, so the list was reloaded", reload the
  list, and move focus by the delete rule. Other failures show `errorText` in a
  toast and leave the row unchanged (the row has no place for a one-line error;
  unlike the line under the rename input, this action has no input).

### Reject

- "Reject…" opens the `ModalFrame` dialog "Reject "{name}"". It has the same
  skeleton as 014's "Delete…" dialog (a body paragraph, secondary "Cancel" with
  initial focus, and danger "Reject"), and the body is one of the two texts in
  "Words" (N videos or 0 videos). The count is `videoCount` from `GET /api/tags`.
  The body says the name can be restored from the rejected names so that, before
  pressing, the user knows that unlike delete there is a way back.
- While running, both buttons are `disabled` and the danger button shows
  `LoaderCircle` (as in the delete dialog).
- On `204`, the dialog closes, the row leaves the list, the toast "Rejected
  "{name}"" appears, and focus follows the delete rule (next row's "Rename" →
  previous row → "New tag"; while "Tentative only" is pressed, "Tentative only"
  replaces "New tag" and stays pressable even after the last tag; see "Toolbar"
  above). The rejected-name list (below) is reloaded.
- `409 tag_not_tentative` (confirmed first from another tab or the API) closes
  the dialog, shows the toast "This tag was already confirmed, so the list was
  reloaded", and reloads the list (edge case "conflicting operations",
  screen-api.md §2). `404 tag_not_found` is handled like today's "the tag no
  longer exists". Other failures show `errorText` in one line of
  `text-sm text-danger` (`role="alert"`) inside the dialog, and the dialog stays
  open (as in the delete dialog).
- "Cancel" and Esc close the dialog without changes and return focus to the
  row's "More actions" (as in the delete dialog).

### Merge

- "Merge into another tag…" on a tentative row keeps the current dialog.
  Tentative tags also appear among the merge-target candidates (with no mark; see
  "Nothing else changes" above). The response's `Tag` (the target) has
  `tentative: false`, and the target row's mark disappears (edge case "merging
  tentative tags", acceptance criterion 11).
- On success, focus goes to the target's name, as today. While "Tentative only" is
  pressed, however, the source is deleted and the target, now confirmed, is not in
  the list (whether the target was tentative or confirmed). Focus then follows
  "Focus when a row leaves the filter" above: "Rename" on the next row at the
  source row's position, otherwise the previous row, and if none remains,
  "Tentative only" (never "New tag").
- The confirmation text does not change. For a tentative source, the sentence
  "\"X\" and its synonyms become synonyms" is still correct (a tentative tag has
  no synonyms, so that part refers to an empty set).

### Rejected names

One collapsible group sits **below** the list (the `divide-y` rows) and its empty
states. It is not shown while loading or after a load failure (the tag list has
never loaded).

- **Heading**: one button inside an `h2` (`text-sm font-medium text-fg`,
  `aria-expanded`). On the left, lucide `ChevronRight` (`size-4`, `rotate-90` when
  open, `motion-reduce:transition-none`), then the text "Rejected names", then the
  count in `text-xs text-fg-muted tabular-nums` ("3"; "0" when empty). The count
  tells, while collapsed, that settled names are kept here. It is not a count of
  tentative tags (so it is not the out-of-scope "notification or badge with the
  number of tentative tags"). `mt-6` separates it from the list above. It starts
  collapsed, and the open state is state of this screen (not stored in the URL or
  `localStorage`).
- **Content** (when open, `mt-2`): one description line in `text-xs
  text-fg-muted`, "Automatic tagging won't create these tags. Remove a name to
  allow it again.", and below it the names (`ul`, `flex flex-wrap gap-1.5`,
  `mt-2`). Each name takes the form of the chips in 014's synonyms dialog (a
  neutral `Chip` — the body surface is `bg`, so `bg-elevated` works — with an ×
  added, `h-6`, `text-xs`). The name is truncated to one line with the full name
  in `title`; the × is a `size-6` square with lucide `X` (`size-3`) and the
  accessible name "Allow "{name}" again". The order is the API's (natural name
  order).
- **Removal**: pressing × sends `DELETE /api/tags/rejected-names?name=…`
  immediately. No confirmation (`UI品質`: an undo-like action may skip
  confirmation). While submitting, that × is `disabled` (`opacity-50`). On `204`
  the chip disappears (a name that was already gone also returns `204`, so the
  result is the same; edge case). No toast (as with removing a synonym; the chip
  disappearing is the result). Focus moves to the next chip's ×, otherwise the
  previous chip's ×, and after the last one to the heading button (the same rule
  as the × on the 014 video page). A failure shows one line below the names in
  `text-xs text-danger` (`role="alert"`), "Couldn't remove "{name}": {reason}",
  and the chip stays. The line clears on the next removal.
- **Loading**: the list is fetched with `GET /api/tags/rejected-names` together
  with the tag list when the screen opens (for the heading count). Until it
  arrives, the heading shows no count, and the open content shows three
  `Skeleton`s (`h-6 w-24`). On failure the heading shows no count, and the open
  content shows one line in `text-xs text-danger`, "Couldn't load the rejected
  names", and a ghost `sm` `Button` "Retry".
- **Reload**: after a reject's `204`, after creating with "New tag", after a
  rename, and after adding a synonym (each can remove a rejected name from the
  list; requirement 15, acceptance criterion 14). The reload happens whether the
  content is open or closed, and an open list updates in place.
- **Empty**: when there are no names, only "No rejected names" in `text-xs
  text-fg-muted` replaces the description line. The heading stays (so the user can
  see where this group is before the first reject).
- Search and "Tentative only" do not affect this group (it is not tags). It is
  also shown in the "No tags yet" empty state (rejected names can exist with zero
  tags).

### States

These rows add to or change the "States" table in 014.

| State | What the screen shows |
| --- | --- |
| "Tentative only" with no tentative tags (search empty) | `EmptyState` (`CircleDashed`) "No tentative tags", description "Tags created by automatic tagging appear here until you confirm or reject them.", `Button` "Show all tags". Pressing it clears the filter and moves focus to "Tentative only" (if clearing leaves zero tags, "Tentative only" becomes `disabled`, so focus goes to "New tag"). After rejecting the last tag, "Tentative only" stays pressed and usable in this state |
| "Tentative only" and search match nothing | `EmptyState` (`SearchX`) "No tentative tags match "{query}"", `Button` "Show all tags". Pressing it clears both the filter and the search and moves focus to the search input |
| Confirm submitting | "Confirm" shows `LoaderCircle` with `aria-disabled` and ignores further presses; the same row's "Rename" and "More actions" are `disabled` |
| Reject submitting | The dialog buttons are `disabled`, "Reject" shows `LoaderCircle` |
| The tag was already confirmed (`tag_not_tentative`) | The dialog closes, toast "This tag was already confirmed, so the list was reloaded", the list reloads |
| Removing a rejected name, submitting | That × is `disabled` (`opacity-50`) |

## Responsive behaviour

Only Tailwind's default breakpoints are used, switched in CSS (library-ui.md 4).
The judged widths are 360px, 768px and 1280px.

| Width | Tag management action row | Tentative row actions | Rejected names | Card chips |
| --- | --- | --- | --- | --- |
| 1280px | Search (`max-w-sm`), "Tentative only", and "New tag" at the right end on one row | Four `IconButton`s on one row; the name column takes the rest | The heading row and wrapping names | Chips widen by the mark; if fewer fit, "+N" grows |
| 768px | Same as above | Same as above | Same as above | Same as above |
| 360px | Search on the first row; "Tentative only" and "New tag" share the second row half and half | The name column, truncated, sits left of the four `IconButton`s (`h-8 w-8`, `gap-1`) and the count column (`w-16`). No horizontal scroll | Names wrap; long names truncate inside the chip | Same as above |

- At 360px the name column of a tentative row is about 92px: 312px (after the
  body's `px-4` and the row's `px-2`) minus two column `gap-2`s, the `w-16` count
  column, the four `IconButton`s (128px) and three `gap-1`s. A confirmed row with
  three actions has about 128px. The name column stays `min-w-0 flex-1`, the name
  `Link` is `truncate` (`min-w-0`), and the mark is `shrink-0` so it stays after
  the name and only the name truncates. `title` shows the full name. No horizontal
  scroll. The implementation PR checks this on a real 360px device (the mark is
  not clipped, and long names do not push out the action group).
- The reject and merge dialogs follow `ModalFrame`'s current width handling (full
  width at narrow widths).

## Review criteria

Judged by looking at a real device (library-ui.md 5). "It is there" alone does
not pass (Q-4).

1. **Visual hierarchy**: with library cards for videos with and without tentative
   tags side by side, the eye goes title → tag name, the dashed circle is noticed
   only after reading the name, and its colour is lighter than the name. Card
   surface, chip surface and text colour are the same for tentative and confirmed
   tags. The same holds on the tag management page: the name stays primary in the
   name column, and the mark is small and after the name. "Confirm" has the same
   weight as the other `IconButton`s, with no accent colour, surface or border.
   "Tentative only" takes the selected form (`accent-soft` surface) only while
   pressed, and when not pressed it is less prominent than the search input
   (`UI品質` "visual hierarchy" and "what does not satisfy the requirement").
2. **Information density**: at 1280×800 on the tag management page, tentative and
   confirmed rows have the same height, and the same number of rows as before
   this feature fits on one screen (014 "Visual review criteria": 12 or more).
   A tentative row adds only the mark and one `IconButton`, with no text label or
   second line. Rejected names take only one collapsed heading line, and no name
   is visible until opened (`UI品質` "information density").
3. **Spacing rhythm**: the spacing between the action row, the count row and the
   list, the rows' `py-2`, the spacing between a card's title and tag row
   (`gap-1`), and between the video page title and tags (`gap-2`) are the same as
   before this feature. Chip heights (`h-5`, `h-6`) are the same; chips only
   widen by the mark. The rejected-names group sits `mt-6` from the list and does
   not look like part of the list rows (`UI品質` "spacing rhythm and
   typography").
4. **Typography**: no new text size or weight. Names have the same format,
   tentative or confirmed, and the mark is an icon only, adding no text. The
   count row "3 of 12 tags" has the same format as during search.
5. **Action priority**: pressing "Tentative only" once lists only tentative tags;
   one press of "Confirm" on each row settles it (and removes it from the list
   while filtering), and focus moves to the next row's "Rename". Rejecting takes
   three presses, "More actions" → "Reject…" → "Reject", two more than
   confirming. A tentative row's menu has no "Delete…", and a confirmed row's
   menu is unchanged (acceptance criterion 12). A rejected name's × removes it in
   one press with no confirmation dialog (acceptance criterion 13).
6. **Clean-up result**: rejecting the tentative tag `高画質` removes its row,
   raises the "Rejected names" count by one, and opening it shows a `高画質` chip
   (acceptance criterion 8). Creating that name with "New tag" removes the chip
   (acceptance criterion 14). Confirming, renaming, adding a synonym to, or
   merging into a tentative tag removes that row's mark and "Confirm" (acceptance
   criteria 7 and 11).
7. **Keyboard**: on the tag management page, Tab goes search → "Tentative only" →
   "New tag" → row name → "Confirm" → "Rename" → "Synonyms" → "More actions" →
   … → the "Rejected names" heading → (when open) each name's ×. `/` moves to
   search. Esc in the reject dialog closes only the dialog.
8. **Guests**: logged out, cards and the video page of public videos show no
   tags, tentative or confirmed, and the tag management page is not reachable
   (acceptance criterion 4).
9. **What does not satisfy the requirement** (`UI品質`): tentative chips or rows
   with a warning colour, a different surface colour or a badge that draws the
   eye before confirmed tags; a count of tentative tags anywhere on the tag
   management page as a notification or badge (the "Rejected names" count is the
   number of rejected names and does not count); needing anything other than
   "Tentative only" (reading every row) to find tentative tags; tentative rows
   with a different height from confirmed rows; rejected names on another screen
   or in a dialog, so that they cannot be checked on the same screen right after
   rejecting.

## Colour

No new pair is used. The mark's `fg-subtle` is used only for icons, never for
text. The pressed form of "Tentative only" (`link` on `accent-soft`) is the same
pair as `IconButton`'s `active` and `SegmentedControl`'s selected form. Rejected
name chips are `fg` on `elevated`; the description and count are `fg-muted` on
`bg`; the error lines are `danger` on `bg` (the tag management body) and `danger`
on `elevated` (the reject dialog; a pair 014 already included in `pairs`).
Nothing is added to `pairs` in `tokens.test.ts`.

## Accessibility

The parent Issue does not ask for screen-reader or contrast design, so only names
are decided here (the same scope as 014).

| Element | Decision |
| --- | --- |
| Mark | An `aria-hidden` icon, plus a visually hidden "Tentative" or "(tentative)" in the accessible name |
| "Tentative only" | `aria-pressed`. The count row reports the filter result through its current `role="status"` (`polite`) |
| "Confirm" | Accessible name "Confirm"; `aria-busy` and `aria-disabled="true"` while submitting |
| Rejected names | The heading button has `aria-expanded`, the content `ul` has `aria-label` "Rejected names", and the × is "Allow "{name}" again". Error lines have `role="alert"` |
