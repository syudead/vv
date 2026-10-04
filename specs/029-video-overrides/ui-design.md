# UI design: Editing the display name and using the current frame as the representative thumbnail on the video page

**Feature**: [parent Issue #517](https://github.com/syudead/vv/issues/517) ·
[plan.md](plan.md) · [contracts/screen-api.md](contracts/screen-api.md) ·
[research.md R-4](research.md#r-4-a-thumbnail-position-is-generated-within-the-request-then-recorded) ·
[R-8](research.md#r-8-the-screen-api-is-two-puts-per-video-an-empty-display-name-and-a-null-position-clear) ·
[R-10](research.md#r-10-display-name-rules-follow-tag-name-rules-with-a-200-code-point-limit)

Sources: the visual rules follow these documents and are not decided again
here.

| Topic | Source |
| --- | --- |
| Colour, interaction states, width breakpoints, playback screen structure | [Library UI](../../docs/design-docs/library-ui.md) (the cards in section 6 on list structure, and section 7 on the playback screen structure) |
| Role tokens | `@theme` in [`web/src/index.css`](../../web/src/index.css). Referred to by name; values are not copied |
| Contrast pairs under test | [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts) |
| Playback screen columns, title format, the two "Video facts" rows with their right-hand actions, the failure line | [specs/012-video-detail-ia/ui-design.md](../012-video-detail-ia/ui-design.md) and the current [`web/src/player/VideoPage.tsx`](../../web/src/player/VideoPage.tsx) and [`VideoFacts.tsx`](../../web/src/player/VideoFacts.tsx) |
| The title-and-tags group, the "Add tag" input and its keys | [specs/014-video-tags/ui-design.md "Video page tags"](../014-video-tags/ui-design.md#video-page-tags) |
| The group name line above the title (the format of a one-line secondary to the title) | [specs/017-folder-groups/ui-design.md "Group line"](../017-folder-groups/ui-design.md#group-line) |
| The visibility toggle (handling of sending and the failure line) | [specs/016-single-account-auth/ui-design.md "Visibility toggle"](../016-single-account-auth/ui-design.md#visibility-toggle) |
| Where strings live and their format | [Screen strings and formatting (i18n)](../../docs/design-docs/i18n.md). The English here is a draft of the intent; after implementation the catalog [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) is canonical |

This feature adds three things to the playback screen (`/videos/:id`), all for
the **owner** only:

1. **Editing** the title (requirements 1 and 2, acceptance criteria 1 and 2).
2. The **file name line** under the title when a display name is set
   (requirement 7, acceptance criterion 7).
3. The **thumbnail item and actions** in the file details row (requirements 5
   and 6, acceptance criteria 5 and 6).

Nothing else changes. List cards, the folder screen, the list view, related
videos and group cards only draw `title`, so the display name appears there
through the API's `title` (requirement 1,
[contracts/screen-api.md §0](contracts/screen-api.md#0-video-changes)). Cards do
not show the file name (the `UI品質` examples of "does not meet the
requirement"). The guest playback screen stays as it is and shows none of the
three (requirement 9). The inside of the player (control bar, layers) does not
change. No new colour, radius or shadow token is added, and nothing is added to
`pairs` in `tokens.test.ts` (see "Colour" below).

## Why here and not elsewhere

The title is edited **in place** (where the `h1` is), not in settings or a
separate window, because the requirement says "completes without leaving the
video page", and the current products compared (renaming in storage, photo and
video management screens) converge on "fix the name where the name is". A
dialog would not let the user check the name at the title's text size, and on
narrow widths it would hide the player.

"Use the current frame as the thumbnail" sits **not in the player's control
bar** but among the actions at the right end of the file details row (next to
"Open file" and "Copy path"), for these reasons:

- A control bar component carries the same weight as playback controls, and
  editing is to be "less prominent than playback controls" (`UI品質` "visual
  hierarchy" and "action priority").
- The control bar is meant to fit on one line even at 360px, and with quality,
  subtitles and speed added there is no room for another control (specs/027
  "Responsive behaviour").
- Current products that let users choose a thumbnail from a frame (video
  publishing services' "use this frame") put that action below the player.
- The result of the choice and the entry point for clearing it (see "Thumbnail
  fact" below) are in the same row, so state, action and failure read in one
  place.

The file name line appears **only when a display name is set** because of
`UI品質` "information density" (the original file name only needs to be
visible, as secondary to the title, when there is a display name). The line's
presence itself marks "this video is overridden" (requirement 7).

## Words

| Place | Text | Notes |
| --- | --- | --- |
| Accessible name and tooltip of the edit button next to the title | Edit name | |
| Accessible name of the edit input | Display name | |
| Placeholder of the edit input | The file-name title itself | User data; not translated |
| Save | Save | |
| Cancel | Cancel | |
| Accessible label and `title` of the file name line | File name | |
| Accessible label, `title` and image `alt` of the thumbnail item | Thumbnail at 1:23 | The time uses the `formatDuration` format |
| Accessible name and tooltip of the button that sets the thumbnail | Use current frame as thumbnail | |
| Accessible name and tooltip of the × that clears it | Use automatic thumbnail | |
| Line when the name could not be saved | Couldn't save the name: {reason} | |
| Line when the thumbnail could not be set or cleared | Couldn't change the thumbnail: {reason} | |
| `display_name_control_characters` | The name can't contain control characters | |
| `display_name_too_long` | The name can't be longer than {limit} characters | |
| `duration_unknown` | The video's length isn't known yet | |
| `thumbnail_position_out_of_range` | The position is past the end of the video | |
| `thumbnail_frame_unavailable` | No image could be made from this frame | |

- The five reasons are added to the `reason` table in
  `web/src/i18n/errors.ts`
  ([contracts/screen-api.md §3](contracts/screen-api.md#3-added-code-and-reason-values)).
  `file_unavailable` and `video_not_found` keep their current text.
- "Thumbnail" and "name" refer to the same things as on list cards (the
  thumbnail) and to title editing (the name). The term "display name" is used
  only as the input's accessible name and does not appear in visible text. To
  the owner it is "the video's name"; the file name line shows the difference
  between display name and file name.

## Title editing

### Placement and weight

- One `IconButton` (`size="sm"`, ghost, lucide `Pencil`) sits to the right of
  the title (`h1`) as the entry point for editing. It shares the `h1`'s row
  with `flex items-start gap-2`; the `h1` is `min-w-0 flex-1` and the button
  `shrink-0`. The button's vertical centre aligns with the **first line** of the
  title (to fit the `h-8` button to the first line of `text-2xl`
  `leading-snug`, about `mt-0.5` at `sm` and above and `-mt-0.5` below). When
  the title wraps, the button stays at the top right and does not drop down.
- Colour is `text-fg-muted`, `text-fg` on hover, the same as "Open file" and
  "Copy path". No surface or border, and no accent colour. It is not noticed
  before the title (`text-xl` to `text-2xl`, `font-semibold`, `text-fg`), and
  is weaker than the tag chips (which have a `bg-elevated` surface) (`UI品質`
  "visual hierarchy").
- Always visible. It is not shown only on pointer hover, because the entry point
  would disappear on devices without hover. Guests do not get it (only the
  `h1`, as today).
- The title format does not change. A display name appears in the same `h1`
  format as a file-name title (`UI品質` "typography").

### Edit mode

Pressing the edit button turns the `h1`'s position into an **input** in place
(the `h1` is not drawn while editing).

- The input is `input[type=text]`. Text size, weight and line height are the
  title's (`text-xl` to `text-2xl`, `font-semibold`, `text-fg`,
  `leading-snug`). Surface `bg-field`, border `border border-border`,
  `rounded-md`, `px-2 py-1`. `-mx-2` aligns the text's left edge with the
  title's left edge (the same idea as the group name line's `-ml-2`). Focus
  gives `border-accent` (the same as the search box and "Add tag"). Width is
  the full column (`w-full`).
- The initial value is the current `title` (the display name if there is one,
  otherwise the file-name title). Focus moves to the input with all text
  selected. The placeholder is `fileTitle`, so emptying the input shows "empty
  = back to the file name" in place (no explanatory sentence is added).
- To the right of the input (below it under `sm`), **Save** (`Button` primary,
  `sm`) and **Cancel** (ghost, `sm`) sit with `gap-2`. They appear only during
  this temporary state, so Save's accent colour does not break "editing is less
  prominent than playback controls". Under `sm`, the input is on the first line
  and the two buttons on the second, aligned left.
- While editing, the file name line under the title (see "File name line"
  below) is hidden; the input's placeholder takes its role. Tags, the
  visibility toggle and the file details stay visible while editing.
- Keys: Enter saves and Esc cancels. While the input has focus, the playback
  screen keys (Space, F, M, C, 0, Esc) do not act (`isEditable` in the current
  `keyboard.ts`; the same as 014 "Add input"). Only Esc pressed outside the
  input closes the screen.
- Cancelling (Cancel or Esc) sends nothing, returns to the `h1`, and returns
  focus to the edit button.

### Save

- Saving sends one `PUT /api/videos/{id}/display-name`
  ([contracts/screen-api.md §1](contracts/screen-api.md#1-put-apivideosiddisplay-name)).
  - When the input value equals the current `title` and no display name is set,
    nothing is sent and it acts as Cancel (no display name identical to the
    file name is created).
  - Empty (including whitespace only) is sent as a clear (`displayName: ""`;
    edge case "empty or whitespace only"). Sending empty for a video without a
    display name returns `200` and changes nothing.
- While sending, the input is `readOnly` and Save is `aria-disabled` (not
  `disabled`, so focus is not lost; the same as 016 "Visibility toggle"), and
  Save's icon becomes lucide `LoaderCircle`
  (`animate-spin motion-reduce:animate-none`). Cancel stays pressable; pressing
  it returns to the `h1` without waiting for the response (a response that
  arrives is used only to replace the video).
- On `200`, the response `Video` replaces the video in `useVideoDetail`, and the
  view returns to the `h1`. The title, the file name line and `document.title`
  take the new values. Focus returns to the edit button. No toast: the changed
  title is the result (acceptance criteria 1 and 2).
- On failure, editing continues. The input text is kept, and one line appears
  **directly below** the row of the input and buttons (`role="alert"`,
  `text-sm text-danger`, led by lucide `AlertCircle` `size-4`) saying
  "Couldn't save the name: {reason}" (the same shape as the could-not-open line
  in 012 "Video facts"). The reason is `errorText` (see "Words" above). The line
  clears on the next save, on cancel, or on moving to another video. No banner,
  toast or dialog.
  - `display_name_too_long` (edge case "too long"): the reason includes the
    `limit` of 200. The input gets no `maxLength`, because the browser counts
    code points differently and pasted text would be cut silently.
  - `404 video_not_found`: the video is refetched (if it is gone, the current
    "video is gone" layer appears).
- When a `video` event on `/api/events` refetches the video during editing (a
  change from another tab or the external API; edge case "concurrent change"),
  the input text is not overwritten. Saving makes this change the last write;
  cancelling shows the refetched title.

## File name line

For a video with a display name set (`displayName` present), one line sits
directly under the title, for the owner only (requirement 7, acceptance
criterion 7).

- Contents from left: lucide `FileVideo` (`size-3.5`, `text-fg-subtle`,
  `aria-hidden`) → `fileTitle` (`text-fg-muted`, truncated to one line,
  `title` set to `File name: {file name}`). The whole line is `text-xs`
  (`text-sm` at `sm` and above). The same format as the group name line:
  smaller than the title and in a secondary colour (`UI品質` "typography" and
  "the original file name is supporting information"). A visually hidden "File
  name" precedes the value for screen readers.
- The row of the `h1` (with the edit button) and this line form one small group
  with `flex flex-col gap-1`, placed inside the current title-and-tags group
  (`gap-2`). The gap between title and file name (`gap-1`) is narrower than the
  gap between title and tags (`gap-2`), so spacing shows that the file name is
  a supplement belonging to the title, not a peer of the tags. The spacing
  outside the group (`gap-2`, `gap-5`) does not change (`UI品質` "spacing
  rhythm").
- Not pressable; neither a link nor a button (Q-5: an element with nothing
  behind it, but the text colour is `fg-muted` with no mark and no hover
  surface, so nothing suggests it can be pressed. Its value is requirement 7
  itself).
- Not shown for videos without a display name. Not shown to guests (guest
  responses have no `displayName` or `fileTitle`).
- The height budget of the left column on wide screens (`lg` and above; the
  player's `max-w-[calc(max(100dvh-17rem,15rem)*…)]`) does not change. In a
  window where this line makes the column overflow, the left column scrolls, as
  with the group name line (012 "`lg` (1024px) and above").

## Thumbnail fact

Two things are added to the file details row (the first row of `VideoFacts`),
for the owner only.

### Item (only when set)

Only when `thumbnailPositionMs` is present, a fourth item sits **after**
length, size and added date.

- Contents from left: lucide `Image` (`size-4`, `text-fg-subtle`,
  `aria-hidden`) → a small image of the current representative thumbnail
  (`img`, `thumbnailUrl`, height `h-6`, 16:9 width, `rounded-sm`,
  `overflow-hidden`, `bg-surface`, `object-cover`) → the position
  (`formatDuration(thumbnailPositionMs)`, `tabular-nums`) → the clearing ×.
  Inside the item `gap-1.5`; between items the current `gap-x-4` (`gap-x-5` at
  `sm` and above).
- The small image lets the user confirm on this screen that "the image became
  the chosen frame". List cards are visible only after going back, and the
  position number alone does not reveal that a black frame was chosen. The
  image's `alt` is "Thumbnail at 1:23". While loading or after a failure it
  stays an empty `bg-surface` box.
- The time is distinguishable from the `Clock` length because the image and the
  `Image` mark sit next to it. Like the other items, it shows no text label; a
  visually hidden "Thumbnail at 1:23" and `title` carry it.
- The clearing ×: a `size-6` square button, lucide `X` (`size-3`),
  `text-fg-muted`, `text-fg` on hover (the same size as the × on tag chips).
  Accessible name "Use automatic thumbnail". Pressing it sends
  `positionMs: null` to `PUT /api/videos/{id}/thumbnail-position`. While
  sending, the × becomes `LoaderCircle` (spinning) and is `aria-disabled`. On
  `200` the item disappears and the image returns to the automatic position
  (acceptance criterion 6).
- When the width is not enough, it wraps like the row's other items.

### Capture button

- An `IconButton` (`size="sm"`, ghost, lucide `Camera`, `text-fg-muted`,
  `text-fg` on hover) sits at the **start** of the group of actions at the
  right end of the details row (`ml-auto`), to the left of "Open file". Its
  accessible name and tooltip are "Use current frame as thumbnail". "Open file"
  and "Copy path" stay next to each other and shift right. For a video without
  a location (which today has no right-hand group), a group with only this
  button is placed when the viewer is the owner and the player is shown.
- Pressing it sends the player's logical playback position at that moment (the
  same value the saved playback position reads), in milliseconds, to
  `PUT /api/videos/{id}/thumbnail-position`. It takes one action whether playing
  or paused, and no time is typed (`UI品質` "action priority", and the plan's
  acceptance criteria). Playback does not stop.
- Pressable when: the player is shown (`showPlayer`); this video's first frame
  has loaded and the logical playback position is settled (after the first
  `loadedmetadata` and after the resume position has been applied; the current
  `PlayerStatus` has no such signal, so implement adds a way for `VideoPlayer`
  to tell the screen); and no state layer (load failure, importing, read
  failure, cannot play, playback failure) and no playback-ended layer (with the
  next-video notice) covers the picture. Otherwise it is `aria-disabled`
  (`opacity-50`). Before the first load, no frame is on screen yet and the
  position is 0 or the value before the resume position is applied, so it would
  send a value different from the frame the user sees. While a layer covers the
  picture there is no "frame currently shown", and the ended position is at
  least the duration and is rejected (contract §2). Once settled, the loading
  spinner while waiting for data, reconnecting, and the central touch controls
  do not cover the picture and keep the position, so the button stays
  pressable.
- While sending (seconds; the response returns only after generation finishes),
  the icon becomes `LoaderCircle` (spinning) and is `aria-disabled`. Pressing
  again sends nothing. Conflicts with another tab or the external API are
  handled by the server's "the image of the last recorded position remains"
  (contract §2).
- On `200`, the response `Video` replaces the video. The item (see "Item"
  above) appears, or its position and image update. No toast: the item's
  changed image and position are the result (acceptance criterion 5). List
  cards read the new `thumbnailUrl` through the `video` notification.
- It is pressable even for a video whose `thumbnailState` is `pending` (the
  import job has not made it yet). Generation happens within the request, and
  the response has `done`.

### Failure

- For both setting and clearing, a failure shows one line directly below the
  details row (above the technical details row; the same place as the
  could-not-open line): `role="alert"`, `text-sm text-danger`, led by
  `AlertCircle` (`size-4`), with the text "Couldn't change the thumbnail:
  {reason}". The reason is `errorText` (see "Words" above).
- The previous item (position and image) is kept. For
  `thumbnail_frame_unavailable`, the server also keeps the previous image
  (edge case "generation fails", contract §2).
- It is never shown together with the could-not-open line; the later one
  replaces the earlier (this place holds one line at a time). It clears on the
  next set, clear or open action, or on moving to another video.
- `404 video_not_found` and `file_unavailable`: the line is shown and the video
  is refetched.

## Responsive behaviour

Only Tailwind's default breakpoints are used, switched in CSS (library-ui.md
4). The widths judged are 360px, 768px and 1280px (the two `lg` columns).

| Width | Title and edit button | While editing | File name line | Thumbnail item and actions |
| --- | --- | --- | --- | --- |
| 1280px | `h-8` button at the top right of the `text-2xl` title | Input, Save and Cancel on one line | `text-sm` | Length, size, added date and thumbnail on one line; three actions at the right end |
| 768px | Same as above | Same as above | `text-sm` | Same as above (fits on one line) |
| 360px | `h-8` button at the top right of the `text-xl` title; the title wraps | Input on the first line; Save and Cancel on the second, aligned left | `text-xs`, truncated to one line | Items wrap; the right-end actions move to the right of the last line. No horizontal scroll |

- The edit button is `shrink-0` and stays at the top right at a pressable size
  (`h-8 w-8`) even with a long title.
- A long display name (edge case): the title wraps and shows in full as today
  (`[overflow-wrap:anywhere]`), and list cards fit it with the current two-line
  truncation (`line-clamp-2`) and `title`. Cards do not change.

## Review criteria

Judged by looking at a real device (library-ui.md 5). "It exists" alone does
not pass (Q-4).

1. **Visual hierarchy**: on opening the playback screen, the eye goes player →
   title → tags, and the edit button and the camera button are noticed after
   that. Both carry the same weight as "Open file" and "Copy path", with no
   accent colour, surface or border. The file name line is clearly smaller and
   lighter under the title and is not read before it. Opening a video with a
   display name next to one without, the title's size, weight and colour do not
   differ (`UI品質` "typography").
2. **Information density**: around the title of a video without a display
   name, everything is as today except the one edit button; no label,
   explanatory sentence or border is added. For a video with a display name,
   only the file name line is added. The details row of a video without a
   chosen thumbnail is as today except the one camera button (`UI品質` "do not
   add to the header's information").
3. **Spacing rhythm**: the gaps between title and tags, between tags and the
   visibility toggle, and between that group and the details row are the same
   as before this feature. The file name line sits close to the title (closer
   than the tags), and the left edge of the input text while editing matches
   the title's left edge. The gap between player and title does not change
   (`UI品質` "spacing rhythm").
4. **Typography**: starting to edit keeps the text size, weight and line height
   of the title, so the change to an input shows only through surface and
   border. When the input is emptied, the file-name title shows faintly as the
   placeholder. The thumbnail item's time uses `tabular-nums`, with the same
   glyphs as the length digits.
5. **Action priority**: pausing and pressing the camera button once makes the
   small image of that frame and its position appear in the details row item a
   few seconds later (acceptance criterion 5). Meanwhile play, seek, volume and
   tag actions all keep working. There is no field for typing a time anywhere.
   One press of × removes the item (acceptance criterion 6). Title editing
   saves with Enter without leaving the screen (acceptance criterion 1).
   Emptying the input and saving returns the title to the file-name title and
   removes the file name line (acceptance criterion 2).
6. **Visible state**: while saving, Save shows a spinner; while setting, the
   camera shows a spinner; while clearing, the × shows a spinner; pressing again
   does not send twice. A failure shows a red line in place (below the input or
   below the details row), and the title, previous image and previous position
   do not change (edge cases "too long" and "generation fails"). No toast or
   dialog appears.
7. **Keyboard**: Tab goes edit button → input (all selected) → Save → Cancel →
   tags → …; Esc cancels inside the input and closes the screen outside it.
   Space, F and M type text inside the input and do not stop playback.
8. **Guest**: after signing out and opening the same video, the title is still
   the display name (acceptance criterion 1), and none of the edit button, the
   file name line, the thumbnail item or the camera button is present
   (acceptance criterion 9).
9. **Examples that do not meet the requirement** (`UI品質`): editing can only
   start in settings or a separate window. Choosing a thumbnail has a time
   input field. The file name appears anywhere on cards, the folder screen, the
   list view, related videos or group cards of a video with a display name. The
   edit or camera button has an accent colour or a surface and is noticed
   before the tags or playback controls.

## Colour

No new pair is used. The input is `fg` on `field` (in `pairs`), the failure
line is `danger` on `bg` (the same as the could-not-open line), and the
secondary line is `fg-muted` on `bg`. `fg-subtle` is used only for marks, never
for text. The small image's backdrop `surface` stands in for the image and
carries no text.

## Accessibility

The parent Issue does not ask for screen reader or contrast design, so only the
names are decided (the same scope as 012).

- The edit button's accessible name: "Edit name". The input's accessible name:
  "Display name".
- The file name line: a visually hidden "File name" precedes the value, and
  `title` carries the same name.
- The thumbnail item: a visually hidden "Thumbnail at 1:23" and the image's
  `alt`. The × is "Use automatic thumbnail". The camera is "Use current frame
  as thumbnail".
- Both kinds of failure line are `role="alert"`. Spinners are `aria-hidden`, and
  elements that are sending are `aria-disabled`.
