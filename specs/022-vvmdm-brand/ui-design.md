# VVMDM brand and screen UI design

This document makes [Issue #414](https://github.com/syudead/vv/issues/414) concrete on screen.
The existing screen structure and interactions follow the [library UI](../../docs/design-docs/library-ui.md)
and each screen's existing UI design. This document defines only the changes for VVMDM. The source
of truth for colors, widths, radii and other implementation values is `@theme` in
`web/src/index.css`. The text and surface contrast checks live in `web/src/theme/tokens.test.ts`.

## Brand shape and placement

The skeleton of the standalone symbol is the ring, the parallel diagonal strokes and the angular
hollow shape in the supplied `logo/symbol.svg`.

- The version with thin capitals in `logo/1.svg` is the standard wordmark. `logo/2.svg` (white)
  and `logo/3.svg` (black) are the single-color versions.
- The PNG files are samples. The app ships outlines derived from the SVG files.
- The 16 px version may optically adjust margins, stroke weight and the gaps between shapes. It
  must not replace the three features or the letterforms with a different mark.

| Place | Display |
| --- | --- |
| Top bar and playback page header | At 640 px and wider, the cyan wordmark is a link to home. At 639 px and narrower, the standalone symbol is used, to keep width for the search box, breadcrumbs and close action. The accessible name of the symbol-only link is "VVMDM home". |
| Setup and sign-in | The wordmark comes before the heading, and the screen names "Create an account" and "Sign in" read as separate headings. On narrow screens the letters are not squeezed, and the size fits the form width. |
| Tab icon | The standalone symbol. At 16, 24 and 32 px, check that the small version keeps the ring hole and the gaps between strokes readable. The brand is identifiable from the shape even in a small tab. Also check the update from a cached old favicon. |
| Version per background | On dark `bg`, `navbar` and `surface`, the cyan version is the default; the white version is used on dark surfaces that need a single color. The black version is for light surfaces. The mark never sits directly on an image or video; it sits on a solid surface. |

The product name is written **VVMDM** in the top bar, authentication, video pages, regular and
video browser titles, and user-facing descriptions. The title is normally "VVMDM". On a video page
it is "{video title} · VVMDM", so a long title stays identifiable by its start in the tab. When
text next to the logo repeats the same word, the decorative side is hidden from assistive
technology. Existing internal keys and CSS names are not display names.

## Colors, type and surfaces

The three supplied dark colors map to roles: black is `navbar`, the deep dark is `bg`, and the
lighter dark is `surface`. The three supplied cyans map to roles: the standard is `accent`, the
light one is `accent-hover`, and the dark one emphasizes press and selection.

- Screens share the role tokens in `index.css`.
- `elevated` separates menus and dialogs from the content surface. `fg`, `fg-muted` and `border`
  are neutral colors readable on these darks.
- Danger, warning and success use the existing semantic tokens, so they stay distinct from the
  cyan of actions.

Strong cyan surfaces are reserved for primary action buttons, the current selection, progress and
keyboard focus, so they share one meaning.

- Regular cards use a dark base, so the video image and title read first.
- Links, selection and progress also use shape, underline, numbers or wording, so meaning does
  not depend on color alone.
- Body text uses `fg` and supporting text `fg-muted`. Decoration and guide lines use `fg-subtle`,
  which keeps body text readable.
- The logo's thin letterforms and the existing body typeface for Japanese and English each have
  their own place.
- Titles set hierarchy with weight and line count; times and counts use aligned digits.
- Card minimum width and video display area keep the current list density.

## Screen composition

### Shell and authentication

The top bar is a solid dark surface with a thin border that separates it from the content. The
logo comes first, then the screen's own search and actions, and the owner's "Refresh" at the
right end.

- The logo has the size and brightness to be recognized at the entry point, and lets the eye move
  on to content and video.
- The current sidebar location shows through surface, text weight and icon. The number of guest
  entry points and the owner-only actions follow the
  [existing authentication design](../016-single-account-auth/ui-design.md).
- The rail and the drawer keep the current location and accessible names.

An authentication screen reads in this order: wordmark, screen name and short description,
inputs, primary button.

- The `surface` form panel stands apart from the background, but inputs are not spaced apart
  just to add room.
- The connection warning appears before the inputs. A failure reason appears next to the field or
  the submit action. Both carry an icon and text.
- While submitting, the label and the disabled state show it.

### Library and folders

In the grid, the thumbnail is the largest surface and the title directly below it is the first
text.

- Duration and progress sit on a near-opaque surface that stays readable over the image. Tags
  and watch status are weaker than the title.
- Regular cards line up on dark surfaces, so the cyan outline on focus and selection is easy to
  find.
- Regular cards and group cards keep the same width and height rhythm. Group cards differ by the
  folder illustration and the video count.
- In list view, the thumbnail and title also read first, and number columns align for comparison.
- Card selection and tag actions keep the meaning in the [library UI](../../docs/design-docs/library-ui.md).

Search is found first, inside the top bar. Filter, sort and view follow as one step weaker.

- At narrow widths, these collapse into the existing "View and sort", while the entry to search
  and filters stays easy to find.
- During selection, the card mark, the selected count and the bottom selection bar confirm it
  together. The bar's main bulk actions show their target count and risk.
- Empty, loading and failed states keep the list's surface rhythm. They state the reason for
  empty and the next action, or the failure reason and "Retry". The loading skeleton previews the
  final card count and density.

The folder page separates folder illustrations, level names and counts from video thumbnails.
Direct videos use the shared card.

- Long Japanese and English names and paths do not overlap actions when wrapped or truncated, and
  the full text stays reachable.
- An empty registered root for the owner shows why it is empty. Guests see only paths that lead
  to public videos.
- Grouping actions sit in the video list context and are less prominent than the main navigation
  actions.

### Tags and settings

The tag list reads as search, create, then existing tags with counts. Each row's rename, synonym
and delete actions are found at the same spacing and alignment.

- An edit field differs from a read-only row by surface and outline. A failure is explained on
  that row.
- The delete and merge confirmations state the result in text. Cancel and confirm differ by
  distance, wording and danger color.
- Settings separate registered folders and import status with headings and groups, so the
  current value, change and delete are not confused.
- Long paths and narrow dialogs never push the primary action out of view.

### Video details and player

On the playback page, the video is the largest surface, then the title and playback controls,
then tags, file information and related videos.

- The header logo is small and does not get in the way of the breadcrumbs or the close action.
- The player uses the brand color where it shows the result of an action, such as playback
  progress and volume. No decorative cyan sits on the video.
- Controls and status text sit on their own surface, readable over bright and dark frames.
- Play and pause switch the button icon and accessible name. Loading, failure and end use the
  existing single status surface with text and the next action.
- Related videos have a density that does not compete with the video. The next video in a group
  keeps the existing preview and cancel action.

## Widths and interaction states

The review widths are **360 px, 768 px and 1280 px**.

| Width | Check |
| --- | --- |
| 360 px | Standalone symbol in the top bar, usable width for search and filters, the drawer, the wrapped selection bar |
| 768 px | Rail and toolbar, multi-column cards |
| 1280 px | Expanded sidebar, grid and list, the related-video column on the playback page |

At the 640 px and 1024 px breakpoints, the existing sidebar and screen layout switches apply, and
primary actions keep usable width. Check the same order as both owner and guest.

| State | Cue |
| --- | --- |
| hover / pressed | Besides the surface change, the pressable area and the result are clear. Devices without hover still show the entry to selection and menus. |
| focus-visible | A continuous `link`-style outline outside the surface, so focus stays visible over a selected fill. The search box highlights only one outer frame. |
| Selected / current | Check mark, count, text weight or current-location surface; never cyan alone. |
| disabled / pending | The reason for disabled, or in-progress text, is nearby, and the state reads even with reduced motion. |
| Empty / failed / danger | Heading, reason and next action in text. Failure and danger add a semantic icon and color. |

With reduced motion, decorative motion stops, and the final state and action cues remain. The
brand expression is part of how existing actions are identified and understood.

## Accessibility and visual review

Check **4.5:1 or higher** for body text against its actual background, and **3:1 or higher** for
key icons, control borders and focus. Check information over translucent layers, thumbnails or
video against real bright and dark images. Judge by eye that the wordmark and symbol read at 16,
24 and 32 px, on dark and light surfaces, and in single color.

Keyboard checks with Tab and Shift+Tab cover:

1. Top bar: menu → logo → search and filters → the owner's refresh.
2. The sidebar.
3. Cards and selection in the list.
4. Tag and settings dialogs.
5. The video header and player controls.

Try Enter / Space, Esc and the player's own keys as the existing design defines them, and check
focus after a close. With a screen reader, listen for "VVMDM home", screen names, card titles and
states, the selected count, submitting and failure announcements, and play, pause and progress.
Check for duplicate logo announcements and unnamed icons.

The implementation review compares every screen in the [quickstart](quickstart.md) at the three
widths above and judges these five points together.

1. **Visual hierarchy**: the library shows video images and titles first, the playback page the
   video and controls. The logo is clear as an entry point but does not take over the content.
2. **Information density**: at the same width, the existing list density holds. Long titles, many
   tags and status indicators do not hide card or row actions.
3. **Spacing rhythm**: the top bar, toolbar, cards and settings groups have consistent spacing
   inside and between them, and groups read even without border lines.
4. **Typography**: the logo's thin capitals and the body text each have their place. Japanese and
   English titles, long paths, times and supporting text differ by size, weight and line height.
5. **Action priority**: search, play and the main save or retry actions read first. Refresh, edit
   and danger actions have strength that fits their context. Every state is distinguishable
   without color.
