# Components rules

Every control, overlay and mark on a new or migrated screen is a component
from `web/src/ui/shadcn`, or one of the vv components in `web/src/ui` listed
below, read through its registry item (`npx shadcn view
./registry/r/<item>.json` from `web/`). `task check` fails on a raw
`<button>`, `<input>`, `<select>`, `<textarea>`, `<table>` or `<dialog>`, a
`role="button"`, or a Radix import outside `web/src/ui`. Each
shadcn component is the shadcn/ui component on the radix base, dressed only
through the tokens of [foundations.md](foundations.md); its structure and
variants are upstream's. A screen composes them; it does not restyle them with
its own colours, radii or heights. The reasons are in
[design-system.md, Components](../../../docs/design-docs/design-system.md#components).

Every PascalCase file in `web/src/ui` is a vv component listed in this file;
no older component is left.

## Shared rules

- **Focus**: every component shows the same ring, the `:focus-visible`
  outline in `web/src/index.css`. Never add `outline-none` or a focus ring of
  your own; `task check` fails on both outside `web/src/ui/shadcn`. Rows inside a menu, a select or a command list show the row that
  has the focus with the `accent` fill instead.
- **States**: hover, pressed, selected and disabled come from the component.
  Do not restyle them per screen; disabled is `opacity-50` everywhere.
  `Button`, `Switch`, `Input` and `RadioGroupItem` draw `aria-disabled="true"`
  the same way (`opacity-50`, the `not-allowed` cursor) but keep their focus
  and their value; the handler still has to ignore the press or the keystroke.
- **Size by density**: the library uses `sm` (`h-8`) controls and `icon-sm`
  icon buttons; the video page's controls below the player are `sm` too, and
  `default` (`h-9`) and `lg` (`h-10`) stay the player's. Icon-only buttons in
  one area share one size.
- **Names**: an icon-only control has an `aria-label` from the i18n catalog and
  a tooltip with the same text.
- **Text**: components take their text from the caller through `t`; they hold
  none of their own.

## Actions and inputs

### Button

Item `button`. An action that happens when pressed.

| Variant             | Use it for                                                                                                                                                                                    |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `default`           | The one main action of an area (a dialog, a form, an empty state); at most one                                                                                                                |
| `outline`           | Secondary actions next to a `default` one, and toolbar triggers (filter, sort, view)                                                                                                          |
| `secondary`         | A quieter filled action where an outline would be too strong                                                                                                                                  |
| `ghost`             | Actions in a dense row: the selection bar, menus' triggers, clearing a field                                                                                                                  |
| `destructive`       | The action that deletes or rejects, named by its verb, in a confirmation                                                                                                                      |
| `ghost-destructive` | A destructive action that stands among plain actions outside a menu, after a `Separator` (clearing a history from its side column); it always opens a `ConfirmDialog`. At most one per screen |
| `link`              | An action that reads as a link inside text                                                                                                                                                    |

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
- `Switch` is a setting that takes effect at once, such as a settings row.
- The video page's visibility control is the one exception: a `secondary`
  `sm` `Button` with `role="switch"` and `aria-checked` whose text is the
  current state (`Private`, `Public`) beside a lock or globe icon, with the
  `primary-soft` fill when public. It keeps the look the video page had before
  the design system.

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
a Combobox for a handful of options (use `Select` or `RadioGroup`). To type a
tag name, use [TagCommand](#tagcommand), which is a `Command` with the tag-name
rules.

### Table

Item `table`. Rows and columns of records, such as the tags in the admin
screen. A screen never uses it bare: it goes in the `DataTable` section of
[patterns.md](patterns.md#sections), which adds the card and its border.

- A row that can be selected starts with a `Checkbox` cell, and the head of
  that column holds the select-all `Checkbox` (`indeterminate` when some rows
  are selected); set `data-state="selected"` on a selected row.
- Row actions sit in the last cell, behind a `ghost` `icon-sm` button that
  opens a `DropdownMenu`.
- Numbers are right-aligned with `tabular-nums`.

Do not use a Table for layout, for a list of cards, or for term and value
pairs (use `FactList`).

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

A toolbar filter whose current value must show without opening anything is a
`FilterDropdownButton` around the menu (see vv components).

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

### Collapsible

Item `collapsible`. Shows or hides a part of a section in place from one
button, such as the member rows of an expandable `CardListItem`, which is
where the showcase draws it. `CollapsibleTrigger` wraps a `Button` through
`asChild`; an icon-only trigger follows the names rule, and its name and
tooltip say what pressing it does now (`Show tasks`, `Hide tasks`). The
hidden part is not rendered, so it holds nothing a reader must find while it
is closed.

Do not use it to switch between views (use `Tabs`), for a list of settings
pages (use `SettingsPage`'s `nav`), or for a floating layer (use `Popover`).

### Sonner

Item `sonner`. Short, passive notices of a finished action ("Added 3 videos to
favorites"). A notice is one line and needs no action, except the undo form
below.

The undo form is a notice of a finished action that can be put back: one
line and one action, "Undo", at its end (`useToast`'s action, Sonner's
`action`). It stays 8 seconds instead of a plain notice's 2.8, and only the
last one shows: a new undo notice dismisses the previous one, so one "Undo"
is on screen and it is the last action's. While the undo runs, the action is
`disabled` with a `Spinner`. Do not give a notice any other action, a second
action, or "Undo" for an action that cannot be put back
.

The app's one `Toaster` sits in `ToastProvider` (`ui/Toast`), and screens send
a notice with its `useToast`, which calls `toast()`. It shows at most three
notices at once on list screens and one at a time on the video page, so a
notice raised in the shell waits its turn after the viewer opens a video. On
list screens it sits under the top bar at the right below `lg`, and at the
bottom centre, above the selection bar, from `lg`; on the video page it sits at
the top centre.

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
rows, a line of text. Give it the size of the content it stands for. Inside
phrasing content, such as a `PageHeader` description (a `p`), pass `asChild`
with a `span`, because a `div` may not sit in a `p`.

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

Item `alert`. A message inside a section, or in a page pattern's `notice`
slot for a condition of the whole page: an error with
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
uses two states: from 640px always `collapsible="icon"` collapsed (the 68px
`sidebar-rail`, each entry a `rail-item` square with its icon above a
`text-rail` label), and a `Sheet` drawer below 640px (`use-mobile`). Entries are `SidebarMenuButton` with `asChild` around a router
link and `isActive` on the current screen; the rail shows the label, so set no
`tooltip`. Account entries go in `SidebarFooter`. Pass `open={false}` so the
sidebar never expands and `keyboardShortcut={false}` so Ctrl/⌘+B stays the
browser's, and show the toggle only below 640px.

An entry that has something waiting for the owner takes a count:
`SidebarMenuCount` (a vv addition to the upstream file) after the
`SidebarMenuButton` in the same `SidebarMenuItem`, with the number to show
(`99+` above 99) and the sentence a screen reader says (`3 need attention`).
On the rail it is a round `destructive-strong` pill with
`destructive-foreground` text at the icon's top-right corner; in the
narrow-width drawer it is `SidebarMenuBadge` at the end of the row in
`destructive`; at 0 it draws nothing. Upstream's `SidebarMenuBadge` alone is
hidden on the icon rail, which vv's rail always is.

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
`VideoThumbnailDuration` bottom right (a versions mark and a public mark may come first),
`VideoThumbnailProgress` on the bottom edge for the share watched,
`VideoThumbnailMark` in a top corner for the selection check (`top-start`) and
a person's `FavoriteToggle` (`top-end`), and `VideoThumbnailNotice` over the whole frame
when the video cannot play. `selected` rings the frame in `primary`.

A video card's credits sit in a `VideoThumbnailLayer`, a frame-sized layer
laid over the thumbnail from outside the card's link, so the face links in it
are not nested in that link; the pointer passes through it except on controls
that set `pointer-events-auto`. In it, `VideoThumbnailCredit` holds faint text
without a panel: the product code at `top-start`, moving right of the
selection check while the check shows, and the performers' `PersonFace`
`mark`s and the label at `bottom-start`.

Do not draw a thumbnail with its own frame, badge or progress bar, and do not
put text other than the duration, the notice and the credits on the image.

### FavoriteToggle

Item `favorite-toggle`. The favorite heart, which is both the mark and the
toggle. `variant="card"` sits on a person's face card, `row` in a
list row and `page`, the heart with its word, in the video page's title row. Video, group and smart
folder cards carry no heart; their list rows and the selection bar do. The heart is filled in
`favorite` when on; when off it shows only on hover and focus of its card or
row. Shown to the owner only.

Do not use the `favorite` colour or a heart for anything else.

### PersonFace

Item `person-face`. A person's face in one of nine frames, or the placeholder
in the same frame, so a row that mixes both keeps one height:

| Shape        | Frame                                       | Where                                                                                                             |
| ------------ | ------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `card`       | Full width, `aspect-square`, `rounded-full` | A card of the people grid                                                                                         |
| `profile`    | Full width, `aspect-portrait`, `rounded-md` | The person page's profile column                                                                                  |
| `chip`       | `size-5`, `rounded-full`, a `text-sm` line  | Next to a name in the video page's cast                                                                           |
| `token`      | `size-6`, `rounded-full`                    | At the start of a chosen person's chip or filter, before the name                                                 |
| `option`     | `size-8`, `rounded-full`                    | At the start of a person's row in a picker's candidate list                                                       |
| `compare`    | `size-8`, `rounded-full`                    | A fetched person in a comparison                                                                                  |
| `judge`      | `size-card-face`, `rounded-full`            | Each side of a row where the owner decides whether two people are the same                                        |
| `scene`      | Full width, `aspect-square`, `rounded-md`   | A face cut from a video, in a grid or a fixed box; square, so it reads as a frame of the video                    |
| `suggestion` | `size-16`, `rounded-full`                   | A person offered as where faces belong, in a ranked list                                                          |
| `mark`       | `size-5`, `rounded-full`, a faint ring      | A performer on a video card's thumbnail; hovering it pops the same face as `profile`, at most `face-popover` wide |

The image is cropped with `object-cover` and `object-face`: centred
horizontally and a third from the top, where a portrait usually holds the
face. The placeholder is lucide `UserRound` in `muted-foreground` on
`bg-muted`, not a letter, since the first character of a Japanese name tells
the viewer little; it also stands in when the image fails to load. Pass
`onAdd` and `addLabel` to make the placeholder a button that adds a face.
Put marks over the frame, such as the favorite heart, as children in a
`PersonFaceMark` (`top-end` or `bottom-end`).

Do not use it for anything but a person: organizations and videos have no
face.

### SpanBar

Item `span-bar`. One lane the length of a video (`h-3`, `bg-muted`,
`rounded-full`) with one block per span at the span's place and width, given
in seconds and clamped to the lane by `spanPlacement`, so a span past the end
is drawn to the end. A block is at least `min-w-1` wide, so a one-second span
stays visible. Pass `onPress` while something can play: each block is then a
`Button` in `primary` with `primary-hover` on hover, the shared focus ring and
`spanLabel` as its name and tooltip. Without it the blocks are plain
`muted-foreground`, so they do not look pressable. `label` names the lane,
such as whose spans it holds.

Do not use it for a progress or a position in the video; those are the
player's bar and `VideoThumbnailProgress`.

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

### TagCommand

Item `tag-command`. A `Command` for typing a tag name, then picking a
matching tag or creating one. The caller filters and orders the options and
passes the tag whose name or synonym matches the spelling exactly. Without
arrow keys, Enter acts on the spelling: an empty name shows the reason, the
exact match is picked, otherwise the create row creates. The name is checked as
it is typed; while it is invalid or a request is in flight, neither Enter nor a
click acts. A paste or drop with a line break is refused with its reason, and
keys pressed during IME composition are ignored. `layout` places the list:

| `layout`   | Use it for                                                                            | List                                                                                                                                                     |
| ---------- | ------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `popover`  | Inside a `Popover` that is the picker: the selection bar's "Add tag" and "Remove tag" | Always shown; the create row first and the first row selected                                                                                            |
| `dropdown` | A small add field in a row of chips: the video page's "Add tag"                       | Opens below the input on focus or typing; Esc, Tab and leaving close it; Esc when closed calls `onEscape`; the create row last                           |
| `inline`   | A target picked inside a dialog: the tag admin's merge target                         | Always open in a fixed-height box in the body (`h-combobox-panel`); `chosenId` marks the picked row; `emptyText` fills an empty box; the create row last |

In `dropdown` and `inline`, no row is selected until the arrow keys or the
pointer select one.

### FilterDropdownButton

Item `filter-dropdown-button`. One filter of a toolbar row of filters that
each show their current value: an `outline` `sm` `Button` holding the
dimension's `name`, its `value` in `text-muted-foreground` and a chevron,
which opens a `DropdownMenu` of `DropdownMenuRadioItem`s (one choice) or
`DropdownMenuCheckboxItem`s (several). Set `active` when the value is not the
default: the button turns `primary-soft` with `primary` text, value included,
so a set filter reads without opening it. A count for a choice goes at the
item's right end in `text-xs` `tabular-nums` (`ml-auto`). Put every filter of
the row in this form, so they share one height, border and type, and follow
them with a `ghost` `sm` `Clear filters` while any is set.

Do not use it for one filter among toggles (use `Toggle`), or when the values
do not need to show on the toolbar (use the library's single filter popover).

### CreditCommand

Item `credit-command`. `TagCommand`'s sibling for the name of a person or an
organization: the same `Command`, layouts and row content (name, hint line,
count at the right, the create row last), with the entity name rules in place
of the tag name rules: 1 to 200 code points and no control characters, a paste
with a line break refused with its reason. Both are one base component,
`NameCommand`, that takes the name rules; screens use `TagCommand` or
`CreditCommand`, never the base. Unlike `TagCommand`, the create row is offered
even when a candidate matches the spelling exactly, so a namesake can be
created as a separate entry, and the caller decides what Enter without arrow
keys picks (`onCommitSpelling`).

`CreditCommand` alone takes an optional second group (`group`): rows under a
heading, after the name results and before the create row in the list below
the input. A row may lead with a `PersonFace option` (`leading`) and draw its
label in `muted-foreground` (`quiet`), as the `Unnamed people` group of the
`Performers` picker does for people without a name. Every person row of a
picker leads with its `PersonFace option`, so a person is chosen by face as
well as by name. A group with no rows draws
no heading.

Do not use either for values other than tag names and the names of people and
organizations.
