# UI design: VVMDM brand and screens

This document makes [Issue #414](https://github.com/syudead/vv/issues/414)
concrete on screen and defines only the changes for VVMDM. The existing screen
structure and interactions follow [Library UI](../../docs/design-docs/library-ui.md)
and each screen's existing UI design. The source of truth for implementation
values such as colours, widths and radii is `@theme` in `web/src/index.css`;
the text and surface contrast checks live in `web/src/theme/tokens.test.ts`.

## Brand shape and placement

The ring, parallel diagonal strokes and angular hollow shape of the supplied
`logo/symbol.svg` form the skeleton of the standalone symbol. The version with
thin capitals from `logo/1.svg` is the standard wordmark; `logo/2.svg` (white)
and `logo/3.svg` (black) are the single-colour versions. The PNGs are samples;
the app serves outlines derived from the SVGs. The 16 px adjustment may
optically correct spacing, stroke weight and the gaps between shapes, but must
not replace the three features or the letterforms with a different mark.

| Place | Display |
| --- | --- |
| Top bar and playback screen header | At 640 px and wider, the cyan wordmark as a link to home. At 639 px and narrower, the standalone symbol, preserving the width of the search field, breadcrumbs and close control. The accessible name of the symbol-only link is `VVMDM ホーム`. |
| Initial setup and login | The wordmark before the heading, with the screen names `初回設定` and `ログイン` readable as separate headings. On narrow screens the letters are not squashed and the mark fits the form's width. |
| Tab icon | The standalone symbol. Check the small-size versions at 16, 24 and 32 px, where the ring's hole and the spacing of the strokes must be readable; even a small tab identifies the brand from the shape. Also check the update from a state where an old favicon is cached. |
| Version per background | On dark `bg`, `navbar` and `surface`, the cyan version by default; on dark surfaces that need a single colour, the white version. For uses on light surfaces, the black version. Never placed directly over images or video; always on a solid surface. |

The product name is written **VVMDM** in the top bar, authentication, the
video screen, the browser's normal and video titles, and user-facing
descriptions. The title is normally `VVMDM`; on the video screen it is
`〈動画の題名〉 · VVMDM`, so the start of a long title stays identifiable in the
tab. When adjacent text repeats the logo's word, the decorative side is hidden
from screen readers. Existing internal keys and CSS names are not treated as
display names.

## Colour, type and surfaces

Of the three specified darks, black takes the `navbar` role, the deep dark the
`bg` role, and the lighter dark the `surface` role. Of the three specified
cyans, the standard one is `accent`, the light one `accent-hover`, and the deep
one the emphasis for pressed and selected states. Screens share the role tokens
in `index.css`. `elevated` separates menus and dialogs from the content
surface, and `fg`, `fg-muted` and `border` are neutrals readable on these
darks. Danger, warning and success use the existing semantic tokens, so their
meaning stays distinct from the cyan of interaction.

Strong cyan surfaces are reserved for primary action buttons, the current
selection, progress and keyboard focus, so they carry one meaning. Ordinary
cards are based on dark surfaces so that the video image and title are read
first. Links, selection and progress also use shape, underline, numbers and
wording, conveying meaning by more than colour. Body text uses `fg`,
supplementary text `fg-muted`, and decoration and guide lines `fg-subtle`,
keeping body text readable. The logo's thin letterforms are kept apart from the
body typeface, which supports the existing Japanese and English text. Titles
get their hierarchy from weight and line count; times and counts from aligned
digits. Card minimum width and video display area keep today's scannability.

## Screen composition

### Shell and authentication

The top bar is separated from the content by a solid dark surface and a thin
border. After the logo come the screen's own search and controls, and at the
right end the owner's `更新`. The logo has the size and brightness to be
recognised at the entrance, and then lets the eye move on to the content and
video. In the sidebar, the current location shows through surface, font weight
and icon; the guest's entry points and owner-only controls follow the
[existing authentication design](../016-single-account-auth/ui-design.md). The
rail and drawer keep the current location and accessible names.

The authentication screen reads in this order: wordmark, screen name and short
description, inputs, primary button. The `surface` form panel stands apart from
the background, but inputs are not spaced out needlessly to fill space. The
connection warning appears before the inputs, and a failure reason near the
affected field or the submit control, with an icon and text. Submitting is
distinguishable by wording and the disabled state.

### Library and folders

In the grid the thumbnail is the largest surface and the title is the first
text directly below it. Duration and progress, even when overlaid on the
image, sit on a near-opaque surface that keeps them readable; tags and watch
state are weaker than the title. Ordinary cards are laid out on dark surfaces so
the cyan outline of focus and selection is easy to find. Ordinary cards and
group cards keep the same width and height rhythm, and group cards are told
apart by the folder artwork and video count. In list view the thumbnail and
title are still read first, and numeric columns are aligned for comparison. The
meaning of existing card selection and tag operations follows
[Library UI](../../docs/design-docs/library-ui.md).

Search is found first, inside the top bar; filters, sort order and view mode
follow as one step weaker. At narrow widths they collapse into the existing
`表示と並び順`, keeping the entry points to search and filtering easy to find.
During selection, the card marks, the selected count and the selection bar at
the bottom confirm it together, and the bar's main bulk actions, target count
and danger are readable. Empty, loading and failure states keep the same
surface rhythm as the list and state in text why it is empty and what to do
next, or why it failed with `再試行`. The loading skeleton previews the card
count and density of the finished list.

The folder screen distinguishes folder artwork, path names and video counts
from video thumbnails, and lists direct videos as the shared cards. Long
Japanese and English names and paths, whether wrapped or truncated, never
overlap controls, and a way to see the whole value remains. An owner's empty
registered root shows why it is empty; guests see only paths that lead to
public videos. Grouping controls sit in the context of the video list and are
less prominent than the main navigation.

### Tags and settings

The tag list reads in this order: search, create, existing tags with counts.
Each row's rename, synonym and delete controls are found at the same spacing
and alignment. An edit field shows its difference from a read-only row by
surface and outline, and a failure is explained on the affected row. Delete and
merge confirmations state the result in text and separate cancel from confirm by
distance, wording and danger colour. In settings, registered folders and import
status are separated by headings and grouping so that the current value,
changes and deletion are not confused. Long paths or narrow dialogs never push
the primary action out.

### Video detail and player

On the playback screen the largest surface is the video, then the title and
playback controls, then tags, file information and related videos. The header
logo is small and does not get in the way of the breadcrumbs and the close
control. The player uses the brand colour where it shows the result of an
interaction, such as playback progress and volume, and never lays decorative
cyan over the video. Control and status text sits on its own surface, readable
over both bright and dark frames. Play and pause switch the button's icon and
accessible name; loading, failure and end use the existing single status
surface to show wording and the next action. Related videos have a density that
does not compete with the video, and the next video of a group keeps the
existing preview and cancel control.

## Widths and interaction states

Review widths are **360 px, 768 px and 1280 px**. Check both owner and guest in
the same order.

| Width | What to check |
| --- | --- |
| 360 px | The standalone symbol in the top bar, the available width of search and filters, the drawer, the wrapped selection bar |
| 768 px | The rail and toolbar, multiple card columns |
| 1280 px | The expanded sidebar, grid and list, the related-videos column on the playback screen |
| 640 px and 1024 px boundaries | Follow the existing switching of sidebar and screen layout, keeping usable width for the main controls |

| State | Cue that distinguishes it |
| --- | --- |
| hover / pressed | Besides the surface change, the clickable area and the result of pressing are clear. On devices without hover, the entry points to selection and menus are visible. |
| focus-visible | A continuous `link`-family outline drawn outside the surface, so focus is visible even over a selected fill. The search field emphasises a single outer border. |
| Selected / current location | Check marks, counts, font weight and the current-location surface; never cyan alone. |
| disabled / pending | The reason for being disabled, or in-progress wording, sits nearby, and the state is conveyed even with reduced motion. |
| Empty / failure / danger | Heading, reason and next action in text; failure and danger add a semantic icon and colour. |

With reduced motion, decorative motion stops and the final state and
interaction cues remain. The brand expression is built into how existing
controls are identified and understood.

## Accessibility and visual judgement

Check **4.5:1 or more** between normal text and its actual background, and
**3:1 or more** for main icons, control borders and focus. Check the
readability of information over translucency, thumbnails or video on real
images, both light and dark. Judge by eye that the wordmark and symbol are
legible at 16, 24 and 32 px, on dark surfaces, light surfaces and in single
colour.

With the keyboard, follow Tab and Shift+Tab through: the top bar menu → logo →
search and filters → the owner's update; the sidebar; the list's cards and
selection; the tag and settings dialogs; the video header and player controls.
Try Enter / Space, Esc and the player's own keys as the existing design
defines, and check where focus goes after closing. With a screen reader, listen
for `VVMDM ホーム`, screen names, card titles and states, the selected count,
submitting and failure announcements, play/pause and progress, and check for
duplicate reading of the logo and unnamed icons.

## Review criteria

The implementation review compares every screen in the [quickstart](quickstart.md)
at the three widths above and judges the following five points together.

1. **Visual hierarchy**: the video image and title in the library, and the
   video and controls on the playback screen, are seen first; the logo is clear
   as an entrance but does not take over the content.
2. **Information density**: at the same width, today's scannability is kept,
   and long titles, many tags and status displays never hide card or row
   controls.
3. **Spacing rhythm**: the top bar, toolbar, cards and settings groups have
   consistent spacing inside and between them, and groups remain readable with
   the borders removed.
4. **Typography**: the logo's thin capitals and the body text are kept apart,
   and Japanese and English titles, long paths, times and supplementary
   information are distinguishable by size, weight and line height.
5. **Interaction priority**: search, playback and the main save or retry are
   understood first; update, edit and dangerous actions have strength matching
   their context. Every state is distinguishable by more than colour.
