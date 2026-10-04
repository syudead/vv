# Foundations rules

Every visual value on a screen is a utility class generated from
`web/src/ui/tokens.css` (registry item `vv-theme`). `task check` fails on a
raw colour, an arbitrary value (`h-[3px]`) and a step outside these scales
(`p-7`, `text-2xl`, `rounded-xl`, `font-bold`), which the theme does not
generate. The reasons are in
[design-system.md, Foundations](../../../docs/design-docs/design-system.md#foundations).

## Colour roles

Pick the colour by the role, never by how it looks. `accent` is the hover and
highlighted-row fill, as in shadcn/ui; the cyan brand colour is `primary`.

| Token                                           | Use it for                                                             |
| ----------------------------------------------- | ---------------------------------------------------------------------- |
| `navbar`                                        | Top bar and sidebar                                                    |
| `background` / `foreground`                     | The page and its body text                                             |
| `muted` / `muted-foreground`                    | Field fills; secondary text, metadata, help text                       |
| `card` / `card-foreground`                      | Cards, list rows, sections                                             |
| `popover` / `popover-foreground`                | Menus, popovers, dialogs, toasts, the selection bar                    |
| `secondary` / `secondary-foreground`            | Secondary buttons, chips, the current sidebar entry                    |
| `accent` / `accent-foreground`                  | Hover and highlighted menu rows                                        |
| `primary` / `primary-foreground`                | The one main action per area, selected marks, progress                 |
| `primary-hover`, `primary-active`               | Hover and pressed states of `primary` fills                            |
| `primary-soft`                                  | Selected rows and active filter chips, with `primary` text             |
| `border`                                        | Dividers and the edges of cards and sections                           |
| `input`                                         | Edges of controls (fields, checkboxes, outline buttons)                |
| `ring`                                          | Keyboard focus                                                         |
| `destructive`, `warning`, `success`             | Text and icons of danger, caution, done, always with words and an icon |
| `destructive-strong` / `destructive-foreground` | The fill of a destructive button and its text                          |
| `*-soft` (`destructive`, `warning`, `success`)  | The fill behind a status message                                       |
| `favorite`                                      | The favorite heart only                                                |
| `overlay`                                       | Behind dialogs and over thumbnails (duration, progress)                |

Cyan (`primary`, `ring`) appears only on actions, selection, focus, progress
and the brand mark. A new text or surface pair is added to the contrast pairs
in `web/src/theme/tokens.test.ts` in the same change.

## Type

| Step        | Use                                                         |
| ----------- | ----------------------------------------------------------- |
| `text-2xs`  | Text on thumbnails: duration, seek time, badges over images |
| `text-xs`   | Metadata, counts, chip labels, help text                    |
| `text-sm`   | Library body, controls, menus, card titles                  |
| `text-base` | Video page body, dialog body                                |
| `text-lg`   | Section headings, dialog titles                             |
| `text-xl`   | Page titles, the video title                                |

Weights: `font-normal`, `font-medium` (controls, labels, card titles),
`font-semibold` (headings). Nothing larger than `text-xl`, and no `font-bold`.

## Spacing and sizes

Spacing and sizes share one 4px scale: `0`, `px`, `0.5`, `1`, `1.5`, `2`, `3`,
`4`, `5`, `6`, `8`, `9`, `10`, `12`, `16`. A layout constant uses its named
step: `navbar`, `sidebar`, `sidebar-rail`, `card-0` to `card-3`,
`list-thumb-cell`, `list-thumb`, `list-number`, `list-number-wide`,
`list-date`, `search-min`, `search-min-sm`, `zoom`, `selection-bar`,
`selection-bar-clearance`, `popover`, `popover-wide`, `chip-label`,
`combobox`, `combobox-list`, `combobox-panel`, `combobox-panel-max`, `menu`,
`detail-aside`, `issue-list`, `folder-list`. Fractions
(`w-1/2`), `full`, `auto` and the container widths (`max-w-md`) are allowed.
A width the scale lacks becomes a named step in `tokens.css`, not an
arbitrary value.

The library (management) is denser than the video page (viewing):

| Role                 | Library  | Video page                        |
| -------------------- | -------- | --------------------------------- |
| Control height       | `h-8`    | `h-9`, `h-10` for the main action |
| Gap between controls | `gap-2`  | `gap-3`                           |
| Gap between cards    | `gap-3`  | `gap-4`                           |
| Section padding      | `p-3`    | `p-4` to `p-6`                    |
| Icons                | `size-4` | `size-4`, `size-5` in the player  |

## Radius, shadow and motion

| Scale  | Steps                                                                                                                                                                  |
| ------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Radius | `rounded-sm` checkboxes and badges; `rounded-md` controls, cards, thumbnails; `rounded-lg` popovers and dialogs; `rounded-full` pills and the scrub dot                |
| Shadow | None on resting surfaces; `shadow-card-hover` on a hovered card; `shadow-elevated` on floating layers; `drop-shadow-mark` on marks over images                         |
| Motion | `animate-fade-in`, `animate-pop-in`, `animate-slide-up`, and `animate-shimmer`, `animate-spin` and `animate-pulse` for loading; each with `motion-reduce:animate-none` |
