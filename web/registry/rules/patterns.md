# Page patterns rules

Every new screen is one page skeleton from `web/src/ui/patterns`, filled with
sections, components and one state block where data is missing. The skeleton
and the sections own every margin, maximum width and gap; the screen passes
content into their slots and writes no spacing classes around them. The
reasons are in
[design-system.md, Page patterns](../../../docs/design-docs/design-system.md#page-patterns).

To build a screen: pick the skeleton from the table below, copy its example
block (`npx shadcn view ./registry/r/<name>-example.json`), then replace the
sample text and data with the screen's own, keeping the structure.

## Choosing the skeleton

| The screen                                                 | Skeleton         | Item               | Example block              |
| ---------------------------------------------------------- | ---------------- | ------------------ | -------------------------- |
| Lists many items the viewer browses, filters and selects   | `ListPage`       | `list-page`        | `list-page-example`        |
| Manages records in rows with bulk actions                  | `AdminTablePage` | `admin-table-page` | `admin-table-page-example` |
| Changes settings, one value per row                        | `SettingsPage`   | `settings-page`    | `settings-page-example`    |
| Shows one item to view, with its facts beside it           | `DetailPage`     | `detail-page`      | `detail-page-example`      |
| Exists only to submit one form (sign-in, first setup)      | `CenteredForm`   | `centered-form`    | `centered-form-example`    |
| Asks for a few values without leaving the current screen   | `FormDialog`     | `form-dialog`      | `form-dialog-example`      |
| Asks the viewer to confirm an action that cannot be undone | `ConfirmDialog`  | `confirm-dialog`   | `confirm-dialog-example`   |

The states of a list or table are shown in `list-states-example`.

Do not nest skeletons, and do not put a skeleton inside a `PageSection` or a
dialog. When no skeleton fits, add one to the design system first, with its
example block and a section here.

## Shared rules

- **Slots, not wrappers**: pass the header, toolbar, band, aside and body
  through the skeleton's props. Do not wrap a slot in a `div` with margins,
  padding, a gap or a maximum width; the skeleton already sets them.
- **No `className` on patterns**: skeletons and sections take no `className`.
  If a screen needs a different spacing, the pattern is wrong for it; change
  the pattern in the design system for every screen, or pick another one.
- **Density**: `ListPage`, `AdminTablePage`, `SettingsPage`, `CenteredForm`
  and the dialogs are library density (`sm` controls, `text-sm`, `p-3` rows).
  `DetailPage` is viewing density: its sections switch to `p-4` rows and
  `text-base` by themselves, and its controls use `default` and `lg`.
- **Text**: patterns hold no text. Every title, label, count and message comes
  from the caller through `t`.
- **One primary action per area**: the header's `actions`, a dialog footer, an
  empty state and a centered form each hold at most one `default` button.

## List page

Item `list-page`. `ListPage` stacks, top to bottom, with `gap-3` and `p-3`
(`p-4` from `sm`):

| Slot           | Put in it                                                         | Never                                    |
| -------------- | ----------------------------------------------------------------- | ---------------------------------------- |
| `header`       | `PageHeader` with the title, the count and the one main action    | Filters or view controls                 |
| `toolbar`      | `Toolbar` with search, filter triggers and the view controls      | The main action; a second row of filters |
| `band`         | The active filters (tag chips), centred under the header          | Controls that are not active filters     |
| children       | `CardGrid` or `DataTable`, then `LoadMoreRow`; or one state block | A heading, a toolbar, page margins       |
| `selectionBar` | `SelectionBar`, only while something is selected                  | Actions that do not use the selection    |

A screen for browsing videos (the library, the folder pages) passes
`toolbarPlacement="topBar"`. Its toolbar then lives in the shell's top bar,
not in the page:

- `toolbar` is a `Toolbar` with `placement="topBar"` wrapped in the shell's
  `TopBarPortal`. It renders nothing in the page.
- `header` is `PageHeader` with `variant="list"`: the title on the left, from
  `sm` in `text-page-title`, and the count at the right end of the same row.
  A folder page instead shows a breadcrumb band fixed under the top bar, with
  the title in an `sr-only` `h1`.
- The page has `gap-4`, `px-3` (`px-4` from `sm`), `pt-4`, and
  `selection-bar-clearance` at the bottom so the last row never hides under
  the selection bar.
- Cards in the `CardGrid` are boxes made with `cardFrameClass`: the `card`
  surface, a `border` edge, `rounded-lg`, the thumbnail across the top
  (`cardThumbnailClass`) and the title inside the box below it, in
  `font-semibold` `text-sm` (`text-base` from `sm`).

The body is the only part that changes between states: the header and the
toolbar stay where they are while the body shows loading, empty or an error.

## Admin table page

Item `admin-table-page`. `AdminTablePage` puts a band between the header and
the table:

| Slot           | Put in it                                                            | Never                          |
| -------------- | -------------------------------------------------------------------- | ------------------------------ |
| `header`       | `PageHeader` with the title, the count and the create action         | Bulk actions                   |
| `band`         | A `TabsList` that narrows the rows, then a `Toolbar` with the search | Controls that act on one row   |
| children       | `DataTable` with a check column, or one state block                  | Cards; a second table          |
| `selectionBar` | `SelectionBar` with the bulk actions, while rows are selected        | Actions that need no selection |

Each row ends with a `DropdownMenu` of its own actions behind an `icon-sm`
ghost button. The destructive item opens a `ConfirmDialog`.

## Settings page

Item `settings-page`. `SettingsPage` is one column, at most `max-w-3xl`, with
the header and then `PageSection`s `gap-8` apart.

- One `PageSection` per topic, titled with a noun ("Library", "Server").
- Inside a section, one `FormRow` per setting, or a `FactList` for values the
  viewer cannot change.
- A setting that acts at once is a `Switch` or a `Select`; an action (scan,
  sign out) is an `outline` `sm` button in its own `FormRow`.

Do not put a save button at the end of the page: each row takes effect when it
changes. A group of values saved together belongs in a `FormDialog`.

## Detail page

Item `detail-page`. `DetailPage` fills the window edge to edge: a sticky header
band `navbar` high with a bottom border, then a main column and an aside
divided by a vertical border. The aside is `detail-aside` wide
(`detail-aside-wide` from `xl`); the columns sit `gap-6` apart inside `px-6`.
Below `lg` everything stacks: the media runs edge to edge under the band, and
the content below it and the aside keep `px-4` (`px-6` from `sm`) side
padding, with a border above the aside.

From `lg`, a screen that keeps the page at the viewport's height (it puts
`DetailPage` in a `lg:h-dvh` flex column) gets a main column that scrolls on its
own and an aside that is a flex column, so a list in it can scroll under its
heading while the header band and the media stay in view. Otherwise the page
scrolls as one under the sticky band.

| Slot     | Put in it                                                                       | Never                   |
| -------- | ------------------------------------------------------------------------------- | ----------------------- |
| `header` | The band's content: the brand or a ghost `Back` button, the location, a close × | A page title block      |
| `media`  | The player or image                                                             | Text or controls        |
| children | The title, tags and controls about the item, then its facts as one icon row     | A card around the facts |
| `aside`  | A small heading and a plain list of related items                               | Main actions; long text |

## Centered form

Item `centered-form`. `CenteredForm` centres one card, at most `max-w-sm`, in
the area around it, with the title, an optional description, the fields
(`Field` with `FieldLabel`) and one submit button as wide as the card. The
`footer` under the card holds a line of help or a link to another such screen.

Use it only for a screen that is nothing but the form. A form inside another
screen is a `FormDialog` or `FormRow`s in a `PageSection`.

## Form dialog

Item `form-dialog`. `FormDialog` is a `Dialog`, at most `max-w-lg` (32rem)
wide, with a title, an optional description, the fields and a footer with
`Cancel` and one `default` submit button named by what it does ("Create",
"Merge"). Below `sm` it spans the screen with a 16px margin.

Keep it to a few fields that belong to one action. Show a field's error with
`FieldError` under it and keep the dialog open; close it only when the action
has succeeded.

- Pass `pending` while the request runs: the submit button and `Cancel` are
  disabled, and the caller ignores closing until the request settles.
- Pass `submitDisabled` while a required value is missing or still being
  counted, and `submitRef` or `cancelRef` to move the focus back to a button
  after a failure. `initialFocus` names the element focused on open; without
  it the first focusable element is.
- Opened without a `trigger`, the dialog returns the focus to the element that
  had it before; an Esc that cancels an IME composition does not close it.

## Confirm dialog

Item `confirm-dialog`. `ConfirmDialog` is an `AlertDialog` that says what will
happen ("Delete "Travel"?"), what it affects, and offers `Cancel` and a
`destructive` action named by its verb ("Delete", "Reject"), never "OK" or
"Yes". Use it only before an action that cannot be undone; an action that can
be undone runs at once and offers its undo in a toast.

When the action is a request, pass `pending`: the action no longer closes the
dialog, both buttons are disabled while the request runs, and the caller
closes the dialog when the request succeeds. Show a failure as a
`destructive` `Alert` in `children` and keep the dialog open.
`actionDisabled` holds the action back until the dialog has what it needs,
such as the count of affected videos.

## Sections

| Section        | Item            | What it is                                                                                                                                                                                                                                                 |
| -------------- | --------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PageHeader`   | `page-header`   | The page title (`h1`, `text-xl`), a count, a one-line description, `leading` (Back, breadcrumb), actions; `titleRef` makes the title focusable for when nothing else can take the focus; `variant="list"` puts the count at the right end of the title row |
| `Toolbar`      | `toolbar`       | Search, filter triggers, then view controls and actions at the end; below `lg` the view controls collapse into a `View and sort` popover; `placement="topBar"` lays them out in the top bar                                                                |
| `PageSection`  | `page-section`  | A titled card (`h2`, `text-lg`); each direct child is one row, divided by a line and padded by the section                                                                                                                                                 |
| `FormRow`      | `form-row`      | A setting: label and description on the left, one control on the right; stacked below `sm`                                                                                                                                                                 |
| `FactList`     | `fact-list`     | Term and value pairs in two columns                                                                                                                                                                                                                        |
| `CardGrid`     | `card-grid`     | Cards `card-0` to `card-3` wide, centred in rows `card-gap` apart; one full-width column below `sm`; `cardFrameClass` makes the card box                                                                                                                   |
| `DataTable`    | `data-table`    | A `Table` on a card with its border; rows, heads and cells come from `table`                                                                                                                                                                               |
| `SelectionBar` | `selection-bar` | The count of selected items, `Clear selection` and the bulk actions, stuck to the bottom of the page                                                                                                                                                       |

- `Toolbar`: put the search in `search`, filter triggers as children, view
  controls (view mode, card size, sort) in `view`, and actions that work
  without a selection in `actions`. Each `view` entry is `{ id, label,
control }`: from `lg` the controls stand inline without a visible name, so
  each needs an `aria-label`; below `lg` the popover shows `label` above each
  control. A control renders in both places, so give it no `id`.
- `Toolbar` with `placement="topBar"`: the search takes the free width (at
  most `max-w-md`) and the controls are centred, `gap-1.5` apart, in `h-9`
  (`inTopBar` on the shared search, filter, sort and view controls). Each
  `view` entry sets `inlineFrom` (`md`, `lg` or `xl`), below which it moves
  into the icon-only `View and sort` popover; `compact` replaces the control
  there, and `hideBelowSm` drops it from the popover below `sm`.
- `PageSection`: put rows directly inside; do not add padding or borders to a
  row. Free text goes in one `<p>`, which becomes one padded row.
- `SelectionBar`: icon-only actions get an `aria-label` and a `Tooltip`; use
  `ghost` `sm` buttons, and a `destructive` action only through a
  `ConfirmDialog`.
  While a bulk action is in flight, disable the actions and pass
  `clearDisabled` so `Clear selection` is disabled too.

## States

Every list, table and section shows its data or exactly one of these blocks,
in the body slot of its skeleton, so the header and the toolbar never move.

| State            | Block                            | Shows                                                                                                |
| ---------------- | -------------------------------- | ---------------------------------------------------------------------------------------------------- |
| Loading          | `LoadingState` (`loading-state`) | `Skeleton` shapes of the final layout: `grid` cards or `table` rows                                  |
| Empty            | `EmptyState` (`empty-state`)     | An icon, a title saying why, a description, and the action that fills it when the viewer can take it |
| Error            | `ErrorState` (`error-state`)     | A `destructive` `Alert`: what failed, and `Retry` when retrying can help                             |
| Loading more     | `LoadMoreRow` (`load-more-row`)  | The content, then a `Spinner` row at the end                                                         |
| Load more failed | `LoadMoreRow` (`load-more-row`)  | The content kept, then an `Alert` row with `Retry`, which requests the same page again               |

```mermaid
stateDiagram-v2
  [*] --> Loading
  Loading --> Content: items
  Loading --> Empty: no items
  Loading --> Error: request failed
  Error --> Loading: Retry
  Content --> LoadingMore: scroll to end
  LoadingMore --> Content: next page
  LoadingMore --> LoadMoreFailed: request failed
  LoadMoreFailed --> LoadingMore: Retry
```

Do not show a spinner over a whole page, replace the content with an error
when only the next page failed, or leave the body blank while loading.
