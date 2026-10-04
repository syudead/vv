# Components rules

Every control, overlay and mark on a new or migrated screen is a component
from `web/src/ui/shadcn`, or one of the vv components in `web/src/ui` listed
below, read through its registry item (`npx shadcn view
./registry/r/<item>.json` from `web/`). `task check` fails on a raw
`<button>`, `<input>`, `<select>` or `<textarea>` outside `web/src/ui`. Each
shadcn component is the shadcn/ui component on the radix base, dressed only
through the tokens of [foundations.md](foundations.md); its structure and
variants are upstream's. A screen composes them; it does not restyle them with
its own colours, radii or heights. The reasons are in
[design-system.md, Components](../../../docs/design-docs/design-system.md#components).

The other PascalCase files in `web/src/ui` (`Button.tsx`, `Popover.tsx`,
`Menu.tsx`, `ModalFrame.tsx`, `Toast.tsx`, `Chip.tsx` and the rest) and
`videoList/FavoriteToggle.tsx` are the components these replace. They stay
only for screens that have not moved yet; new code never imports them.

## Shared rules

- **Focus**: every component shows the same ring, the `:focus-visible`
  outline in `web/src/index.css`. Never add `outline-none` or a focus ring of
  your own. Rows inside a menu, a select or a command list show the row that
  has the focus with the `accent` fill instead.
- **States**: hover, pressed, selected and disabled come from the component.
  Do not restyle them per screen; disabled is `opacity-50` everywhere.
- **Size by density**: the library uses `sm` (`h-8`) controls and `icon-sm`
  icon buttons; the video page uses `default` (`h-9`) and `lg` (`h-10`) for its
  main action. Icon-only buttons in one area share one size.
- **Names**: an icon-only control has an `aria-label` from the i18n catalog and
  a tooltip with the same text.
- **Text**: components take their text from the caller through `t`; they hold
  none of their own.

## Actions and inputs

### Button

Item `button`. An action that happens when pressed.

| Variant       | Use it for                                                                           |
| ------------- | ------------------------------------------------------------------------------------ |
| `default`     | The one main action of an area (a dialog, a form, an empty state); at most one       |
| `outline`     | Secondary actions next to a `default` one, and toolbar triggers (filter, sort, view) |
| `secondary`   | A quieter filled action where an outline would be too strong                         |
| `ghost`       | Actions in a dense row: the selection bar, menus' triggers, clearing a field         |
| `destructive` | The action that deletes or rejects, named by its verb, in a confirmation             |
| `link`        | An action that reads as a link inside text                                           |

Sizes are `sm`, `default` and `lg`, and `icon-sm` and `icon` for icon-only
buttons. Combine an icon-only button with `Tooltip`; put a link that looks like
a button through `asChild` (`<Button asChild><Link /></Button>`), so it stays a
link.

Do not use a Button for navigation that is a plain link in text, to show a
state that stays on (use `Toggle`), or to pick one of several options (use
`ToggleGroup` or `RadioGroup`). Do not put two `default` buttons in one area.

### Input, Textarea, Label and Field

Items `input`, `textarea`, `label`, `field`. Text the viewer types.

- `Input` takes one line (a name, a search, a token); `Textarea` takes
  several lines and grows with its content.
- Every field has a visible `Label` bound with `htmlFor`, or, in a toolbar
  where the placeholder names it, an `aria-label`.
- Wrap a form field in `Field` with `FieldLabel`, the control,
  `FieldDescription` for help and `FieldError` for the reason it is wrong; set
  `aria-invalid` on the control at the same time. Group related fields in
  `FieldGroup`, or in `FieldSet` with `FieldLegend` when they answer one
  question.
- `Field orientation="horizontal"` puts a `Checkbox` or `Switch` before its
  label; a settings row (label and description on the left, control on the
  right) also uses it, with `FieldContent` around the text.

Do not use `Input` for a choice among known values (use `Select`,
`RadioGroup` or `Combobox`), or a `Textarea` for a single line.

### Select and RadioGroup

Items `select`, `radio-group`. Pick one value from a fixed list.

- `RadioGroup` when the options are few (up to about six) and seeing all of
  them helps the choice, as for the sort order in the narrow toolbar. Each
  `RadioGroupItem` has a `Label`.
- `Select` when the list is longer, or space is short and the current value is
  what matters, as in a settings row.

Do not use either for a choice that acts at once in a toolbar (use
`ToggleGroup`), for many values the viewer would search (use `Combobox`), or
for an on and off setting (use `Switch` or `Checkbox`).

### Checkbox and Switch

Items `checkbox`, `switch`. Turn one thing on or off.

- `Checkbox` is a choice inside a form, a filter popover or a selection, and
  takes effect with the rest of it. It has an `indeterminate` state for a
  group partly selected. A card's selection mark is a `Checkbox` with an
  `aria-label` that names the item.
- `Switch` is a setting that takes effect at once, such as a video's
  visibility.

Pair either with a `Label` (`Field orientation="horizontal"`). Do not use a
`Switch` inside a form that is saved with a button, or a `Checkbox` to run an
action.

### Toggle and ToggleGroup

Items `toggle`, `toggle-group`. Buttons that stay pressed.

- `Toggle` is one state that stays on: an applied filter chip, which the
  viewer unpresses to remove. Its name is the thing it applies (the tag name),
  and its `title` says that pressing removes it.
- `ToggleGroup type="single"` is an exclusive choice that acts at once: view
  mode, sort direction. Its items are radios to assistive technology. Use
  `variant="outline"` and the area's size; icon-only items get an
  `aria-label` and a `Tooltip`.
- `ToggleGroup type="multiple"` is a row of independent toggles, such as text
  formatting.

The pressed state is `primary-soft` with `primary` text, the same as a
selected row. Do not use a Toggle for a one-off action (use `Button`) or for a
choice inside a form that is saved later (use `RadioGroup` or `Checkbox`).

### Slider

Item `slider`. Pick a value on a range by dragging or with the arrow keys.

Use it for a value whose exact number does not matter, such as the card size
(four steps). Give it an `aria-label`; the label goes on the thumb, which takes
the focus. Do not use it when the viewer must hit an exact value (use an
`Input` or a `Select`).

### Combobox and Command

Items `combobox`, `command`. Pick one value from many by typing.

- `Combobox` is a button showing the current value; it opens a `Command` in a
  popover with a search field and the matching options. Use it for a long
  list (folders, tags) where typing is faster than scrolling.
- `Command` is the filtered list on its own. Put it inside an existing
  popover when the popover itself is the picker, such as a selection bar's
  "Add tag" and "Remove tag"; pass `shouldFilter={false}` when the caller
  orders and filters the options. Give `Command` and `CommandList` a `label`
  from the catalog; cmdk's own default names are English text outside it.

Enter takes the selected row. When typing can create a value, put the create
row first, so Enter creates unless the viewer moved to another row. Do not use
a Combobox for a handful of options (use `Select` or `RadioGroup`).

## Overlays and feedback

### Dialog

Item `dialog`. A modal window for a task that needs the viewer's full
attention: a form (create, merge, edit synonyms) or a choice from a long list.
Compose `DialogHeader` (`DialogTitle`, `DialogDescription`), the body, and a
`DialogFooter` with `Cancel` and at most one primary action. The window is at
most `max-w-lg` and keeps a 16px margin on narrow screens.

Do not use it to confirm a destructive action (use `AlertDialog`), to show a
short hint (use `Tooltip`) or for options next to a control (use `Popover`).

### AlertDialog

Item `alert-dialog`. Confirms an action that cannot be undone, such as
deleting or rejecting a tag. Say what happens in `AlertDialogTitle` and
`AlertDialogDescription`, put `AlertDialogCancel` first and name
`AlertDialogAction` by its verb with `variant="destructive"`. It does not
close on an outside click.

Do not use it for a reversible action, which needs no confirmation, or for a
form (use `Dialog`).

### Popover

Item `popover`. A non-modal panel anchored to its trigger, for a few options
or a short form that belongs to that control: view and sort, adding a tag to
the selection, the scan summary. Default width `w-popover`; pass `container`
when the trigger sits in a full-screen element. Combine with `PopoverHeader`
for a title.

Do not use it for a list of actions (use `DropdownMenu`) or for a task that
needs the whole screen (use `Dialog`).

### DropdownMenu

Item `dropdown-menu`. A list of actions or one choice from a few, opened from
a button: visibility, favorites, sort order. Use `DropdownMenuItem` for
actions, `DropdownMenuRadioGroup` with `DropdownMenuRadioItem` for one choice
and `DropdownMenuCheckboxItem` for toggles. A destructive item uses
`variant="destructive"` and sits after a `DropdownMenuSeparator`.

Do not put inputs in it (use `Popover`) or use it for navigation between
screens (use `Sidebar` or links).

### Tooltip

Item `tooltip`. Names an icon-only control or adds a short hint, shown on
hover and keyboard focus. One `TooltipProvider` wraps the app. The text is one
short line; a `Kbd` may follow it.

Do not put interactive content or anything the viewer must read to act in it;
touch screens never show it.

### Tabs

Item `tabs`. Switches between views of the same content in place, such as the
tag admin lists. `TabsList` with `variant="default"` is the filled segment;
`variant="line"` is the underlined row for page-level tabs. A count follows
the label in `text-muted-foreground`.

Do not use it to change a setting (use `ToggleGroup`) or to move to another
screen (use links).

### Sonner

Item `sonner`. Short, passive notices of a finished action ("Added 3 videos to
favorites"). Call `toast()` from `sonner` under one `Toaster` for the app. A
notice is one line and needs no action.

Until every screen moves, the app's notices still go through `useToast` from
`ui/Toast`, which also queues notices one at a time on the video page. The app
switches to `Toaster` in one change, so keep calling `useToast` in existing
screens.

Do not use it for errors the viewer has to fix (use `Alert` near the cause) or
for anything that must stay on screen.

### Badge

Item `badge`. A small label: a tag chip, a count, a status. `secondary` for
tags, `soft` for a selected or active chip, `default` for a count that needs
the brand colour, and `destructive`, `warning`, `success` for status, always
with a word and an icon. With `asChild` it can wrap a link; a removable chip
puts its `×` button inside.

Do not use it as a button for a primary action or for long text; the label is
one short phrase.

### Skeleton

Item `skeleton`. The final layout's shapes while content loads: cards, list
rows, a line of text. Give it the size of the content it stands for.

Do not cover a whole page with a spinner instead, or use it for content that
loads faster than the eye notices.

### Progress

Item `progress`. How far a known amount of work has come: a scan's videos, the
part of a video watched. Pass `value` and `max`, and `aria-label`. Without a
`value` it is indeterminate.

Do not use it for a wait with no measure; use `Spinner` for that.

### Spinner

Item `spinner`. A short wait with no measure: loading more items at the end of
a list, a pressed action that is still running. It carries the accessible name
`Loading`.

Do not use it in place of a `Skeleton` for the first load of a list.

### Alert

Item `alert`. A message inside a section: an error with
`variant="destructive"`, a warning (stalled playback) with `warning`, done
with `success`, or neutral guidance (autoplay is off) with the default. Start
with an icon, then `AlertTitle` and `AlertDescription`; put a `Retry` in
`AlertAction` when retrying can help.

Do not use it for a passing notice (use `Sonner`) or to fill an empty list
(use `Empty`).

### Empty

Item `empty`. The empty state of a list or section: an icon in `EmptyMedia`
(`variant="icon"`), one line in `EmptyTitle` saying why it is empty, and in
`EmptyContent` the action that fills it when the viewer can take it.

Do not use it while loading (use `Skeleton`) or after a failure (use `Alert`).

### Separator

Item `separator`. A hairline between groups of controls or sections.
Horizontal by default, `orientation="vertical"` inside a row.

Do not add separators where spacing already separates the groups.

### Kbd

Item `kbd`. A key or a search operator in running text or a tooltip (`tag:`,
`-`). Group several with `KbdGroup`.

Do not use it for anything the viewer cannot type.

### Breadcrumb

Item `breadcrumb`. The path to the current folder. `BreadcrumbLink` (with
`asChild` around a router link) for each ancestor, `BreadcrumbPage` for the
current one, `BreadcrumbSeparator` between them, `BreadcrumbEllipsis` when the
middle is collapsed.

Do not use it for a flat set of filters or for steps of a task.

### Sidebar

Item `sidebar`. The app's main navigation, from `SidebarProvider` down. vv
uses three states: expanded at wide widths, `collapsible="icon"` (the rail,
labels in tooltips) when collapsed, and a `Sheet` drawer below 640px
(`use-mobile`). Entries are `SidebarMenuButton` with `asChild` around a router
link, `isActive` on the current screen and `tooltip` set to the label; account
entries go in `SidebarFooter`. Pass `open` and `onOpenChange` to keep the
viewer's choice.

Do not add a second sidebar or put page controls in it; they belong in the
toolbar.

### Sheet

Item `sheet`. A panel from the screen's edge. In vv it is only the sidebar's
narrow-width drawer; `Sidebar` renders it.

Do not use it for forms or details; use `Dialog`.

## vv components

### VideoThumbnail

Item `video-thumbnail`. The 16:9 thumbnail frame shared by video, group and
folder cards and list rows. Put the image (or `VideoThumbnailImage`, or a
hover preview) as children, then the overlays it needs:
`VideoThumbnailDuration` bottom right (a public mark may come first),
`VideoThumbnailProgress` on the bottom edge for the share watched,
`VideoThumbnailMark` in a top corner for the selection check (`top-start`) and
`FavoriteToggle` (`top-end`), and `VideoThumbnailNotice` over the whole frame
when the video cannot play. `selected` rings the frame in `primary`.

Do not draw a thumbnail with its own frame, badge or progress bar, and do not
put text other than the duration and the notice on the image.

### FavoriteToggle

Item `favorite-toggle`. The favorite heart, which is both the mark and the
toggle. `variant="card"` sits in a `VideoThumbnailMark` on a card, `row` in a
list row and `page` in the video page's action group. The heart is filled in
`favorite` when on; when off it shows only on hover and focus of its card or
row. Shown to the owner only.

Do not use the `favorite` colour or a heart for anything else.

### TentativeMark

Item `tentative-mark`. The dashed circle after a tentative tag's name, in a
`Badge` (`size="chip"`) or an admin row (`size="row"`). It is decorative; the
element around it says "tentative" to screen readers.

Do not use it for anything other than tentative tags.

### ScrubPreview

Item `scrub-preview`. The card scrub: a band at the bottom fifth of a card's
thumbnail that shows the frame under the pointer (`useScrubPreview`,
`ScrubBand`, `ScrubFrame`). It goes inside a `VideoThumbnail`.

Do not confuse it with the video page's seek preview over the player's
progress bar, which is a `special` exception in `index.css`.

### ThumbnailBackdrop

Item `thumbnail-backdrop`. A blurred copy of a portrait thumbnail behind it,
so a 9:16 video fills its 16:9 frame. Put it first inside the frame, with the
image above it.

Do not use it for landscape images, which fill the frame already.

### BrandHomeLink

Item `brand-home-link`. The VVMDM logo in the top bar, linking to the library:
the symbol below the `sm` width and the wordmark above it. One per screen.

Do not use the logo images anywhere else.
