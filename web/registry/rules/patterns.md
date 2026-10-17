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

| The screen                                                  | Skeleton         | Item               | Example block              |
| ----------------------------------------------------------- | ---------------- | ------------------ | -------------------------- |
| Lists many items the viewer browses, filters and selects    | `ListPage`       | `list-page`        | `list-page-example`        |
| Manages records in rows with bulk actions                   | `AdminTablePage` | `admin-table-page` | `admin-table-page-example` |
| Changes settings, one value per row                         | `SettingsPage`   | `settings-page`    | `settings-page-example`    |
| Shows one item to view, with its facts beside it            | `DetailPage`     | `detail-page`      | `detail-page-example`      |
| Exists only to submit one form (sign-in, first setup)       | `CenteredForm`   | `centered-form`    | `centered-form-example`    |
| Asks for a few values without leaving the current screen    | `FormDialog`     | `form-dialog`      | `form-dialog-example`      |
| Asks the viewer to confirm an action that cannot be undone  | `ConfirmDialog`  | `confirm-dialog`   | `confirm-dialog-example`   |
| Works on one image that needs the whole window              | `EditorDialog`   | `editor-dialog`    | `editor-dialog-example`    |
| Fills in many values saved together, on a screen of its own | `FormPage`       | `form-page`        | `form-page-example`        |
| Walks through fixed steps one at a time (the first setup)   | `StepsPage`      | `steps-page`       | `steps-page-example`       |

The states of a list or table are shown in `list-states-example`.

Do not nest skeletons, and do not put a skeleton inside a `PageSection` or a
dialog; the one exception is the `ConfirmDialog` an `EditorDialog` opens before
closing. When no skeleton fits, add one to the design system first, with its
example block and a section here.

## Shared rules

- **Slots, not wrappers**: pass the header, toolbar, band, aside and body
  through the skeleton's props. Do not wrap a slot in a `div` with margins,
  padding, a gap or a maximum width; the skeleton already sets them.
- **No `className` on patterns**: skeletons and sections take no `className`.
  If a screen needs a different spacing, the pattern is wrong for it; change
  the pattern in the design system for every screen, or pick another one.
- **Density**: `ListPage`, `AdminTablePage`, `SettingsPage`, `CenteredForm`,
  `StepsPage` and the dialogs are library density (`sm` controls, `text-sm`, `p-3` rows).
  `DetailPage` is viewing density: its sections switch to `p-4` rows and
  `text-base` by themselves, and its controls use `default` and `lg`.
- **Text**: patterns hold no text. Every title, label, count and message comes
  from the caller through `t`.
- **One primary action per area**: the header's `actions`, a dialog footer, an
  empty state, a centered form and a steps page's footer each hold at most one
  `default` button.

## List page

Item `list-page`. `ListPage` stacks, top to bottom, with `gap-3` and `p-3`
(`p-4` from `sm`):

| Slot           | Put in it                                                                                   | Never                                                                   |
| -------------- | ------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| `header`       | `PageHeader` with the title, the count and the one main action                              | Filters or view controls                                                |
| `toolbar`      | `Toolbar` with search, filter triggers and the view controls                                | The main action; a second row of filters                                |
| `band`         | The active filters (removable tag chips), when any apply                                    | Controls that change the view                                           |
| `aside`        | Tools that narrow the body and are read after it, such as a `JumpList`; at most one section | The body's content, the main action, filters that belong on the toolbar |
| children       | `CardGrid`, `DataTable` or `GroupedList`, then `LoadMoreRow`; or one state block            | A heading, a toolbar, page margins                                      |
| `selectionBar` | `SelectionBar`, only while something is selected                                            | Actions that do not use the selection                                   |

`toolbarRow="header"` puts the header and the toolbar on one row from `lg`,
the header at its own width and the toolbar filling the rest, for a page
whose title is short and whose toolbar is a filter and a search; below `lg`
they stack. `aside` is a column at the right of the body from `lg`,
`list-aside` wide (the named utility `grid-cols-list-aside`), stuck under the
top bar and scrolling inside itself when taller than the viewport; below
`lg` it is drawn full width between the band and the body, and the section
in it draws its own narrow form. The header, the toolbar and the aside stay
where they are in every state of the body.

A list people browse (the library, folder pages) puts its toolbar in the
shell's top bar: pass a `Toolbar` with `placement="topBar"` wrapped in the
shell's `TopBarPortal` as `toolbar`. The portal draws nothing in the page, so
the page starts with the header. The body is the only part that changes
between states: the header and the toolbar stay where they are while the body
shows loading, empty or an error.

## Admin table page

Item `admin-table-page`. `AdminTablePage` is one column, at most `max-w-4xl`,
centered. The header and the band stick under the top bar, so the create
action, the tabs and the selection stay in reach while the rows scroll; pass
`bandRef` to measure the stuck band, pass its bottom to the `DataTable`'s
`stickyHeaderTop` so the column header sticks right under it, and keep a
focused row clear of both:

| Slot           | Put in it                                                                                                                                      | Never                           |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------- |
| `toolbar`      | A `Toolbar` with `placement="topBar"` in the shell's `TopBarPortal`                                                                            | The create action               |
| `header`       | `PageHeader` with the title, the count and the create action; while rows are selected, a `SelectionBar` with `placement="header"` in its place | Bulk actions in the page header |
| `band`         | A `TabsList` that narrows the rows, then the active filter chips                                                                               | Controls that act on one row    |
| children       | `DataTable` with a check column, or one state block                                                                                            | Cards; a second table           |
| `selectionBar` | Only a bottom `SelectionBar` when the header cannot hold it                                                                                    | Actions that need no selection  |

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
changes. A group of values saved together belongs in a `FormDialog`, or in a
`FormPage` when it is more than a few.

### Page list

A settings area of several pages passes the list of its pages in `nav`: a
`SettingsNav` named with the area's title, holding one `SettingsNavItem` per
page, each wrapping one router link, with `current` on the open page. The
skeleton then becomes `max-w-4xl` wide, and one markup serves every width;
CSS decides what shows, not `useIsMobile`.

| Width      | `view="list"` (the area's own address)                                                                                                                                                                              | `view="page"` (a page's address)                       |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| From `sm`  | `grid-cols-settings-page`: the list a column `settings-nav` wide at the left, sticky under the top bar (`top-navbar`) and scrolling inside itself past `settings-nav-max`, `gap-6` from the header and the sections | The same                                               |
| Below `sm` | The list alone; its heading is the page's title (`text-xl`)                                                                                                                                                         | The header and the sections alone, under the back link |

- The list's rows are `h-8`, `px-2`, `text-sm`, `text-muted-foreground`, with
  the hover fill `bg-accent`; the current row carries `aria-current="page"`
  and, from `sm` only, the sidebar's current mark (`bg-secondary`,
  `text-foreground`, `font-medium`). The rows carry no icons.
- The back link to the list is a `ghost` `sm` `Button` around a router link,
  with `ArrowLeft` and the list's title, in the `PageHeader`'s `leading`.
  With `nav`, `leading` holds only that link: the skeleton hides it from
  `sm`, where the list is in view.
- The page title is the list entry's word, with no description under it.

Without `nav` the skeleton is one column at most `max-w-3xl`, as above.

### Page notice

A condition that applies to the whole page, such as a setting the page
depends on that is not saved yet, goes in `notice`: one `Alert` between the
header and the sections, `gap-6` from both. Without `notice` nothing is drawn.

- Use the default `Alert` with an icon and `role="note"` for a condition the
  viewer resolves elsewhere, with a text link to that page; use
  `destructive`, with `Retry` in `AlertAction`, when reading the condition
  failed.
- When the condition locks the page's controls, keep them in place with their
  values visible and set `aria-disabled="true"` on each; `Button`, `Switch`
  and `Input` draw it as disabled. The attribute stops nothing by itself, so
  each handler returns without changing the value or sending a request.
- Say the reason once, in the notice, not on each row.

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
"Merge"). Below `sm` it spans the screen with a 16px margin. On a short
screen only the fields scroll; the title and the footer stay in the dialog.

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
- Pass `size="wide"` for a form that sets values side by side for comparison:
  the dialog stops at `max-w-2xl` (42rem) instead of `max-w-lg`, and below
  `sm` it still spans the screen with the 16px margin.
- Leave out `cancelLabel` for a last step whose only action is closing, such
  as `Done` after a result; the footer then holds only the submit button.

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

## Editor dialog

Item `editor-dialog`. `EditorDialog` is a modal `Dialog` that fills the
viewport edge to edge on `bg-background`. A header band `navbar` high with a
bottom border holds the title, a muted detail after it and the close `×` (a
`ghost` `icon-sm` `Button`). Under it sit a `stage` and an `aside`:

| Width      | Layout                                                                                                                                                                            |
| ---------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| From `lg`  | The stage fills the rest; the aside is a column `editor-aside` wide (`editor-aside-wide` from `xl`) with a left border, scrolling on its own, and its `footer` sits at its bottom |
| Below `lg` | The stage sits under the band, `editor-stage` high (about 45% of the viewport); the aside follows it, the body scrolls as one, and the `footer` sticks to the viewport's bottom   |

| Slot     | Put in it                                                                                                             | Never                          |
| -------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------ |
| `stage`  | The image being worked on, fitted to the stage; the stage centres it on `bg-muted` with `p-4`                         | Text fields or long text       |
| `aside`  | The inputs, top to bottom in the order they are filled, each with at most one short hint, then the status of the work | The main actions               |
| `footer` | The main actions, the one `default` first and as wide as the aside                                                    | More than one `default` button |

Esc and the `×` call `onClose`; the screen closes the dialog itself, or first
asks with a `ConfirmDialog` when closing would lose work. An Esc that cancels
an IME composition does not close it. On close the focus returns to the element
that had it before, and `initialFocus` names the element focused on open.

Use it for a task on one image that needs the whole window, such as painting
over a video frame or comparing two images. A form stays a `FormDialog`.

## Form page

Item `form-page`. `FormPage` is a page of its own, not a dialog, that fills
the window on `bg-background` for a form of many values saved together. A
header band `navbar` high, sticky at the top at every width, with a bottom
border and `bg-background/90 backdrop-blur-md`, holds the `title` (the page's
`h1`, `text-lg font-semibold`), a muted `detail` after it (`text-sm`,
truncated, hidden below `sm`) and the `actions` at its right end. Under the
band sit the `notice` and then the body, `px-4` (`px-6` from `sm`) and `py-6`.

| Width      | Layout                                                                                                                                                                                                         |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| From `lg`  | `grid-cols-form-page`: the `aside` a column `form-page-aside` wide at the left, then the two panels in the ratio 5:6, `gap-6` apart. The page keeps the viewport's height and each panel scrolls inside itself |
| Below `lg` | The aside and the panels stack in that order, `gap-6` apart, and the page scrolls as one under the band                                                                                                        |

| Slot      | Put in it                                                                                                                               | Never                                         |
| --------- | --------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------- |
| `actions` | `Cancel` (`ghost` `sm`) then the one `default` `sm` button that saves                                                                   | A close `×`, a logo or a breadcrumb           |
| `notice`  | The failure line of the save (`role="alert"`, `text-sm text-destructive`), so it stays under the band that holds the save               | Field errors; they go under their field       |
| `aside`   | What is being edited, read only: an image, a name and its facts, and the link back; named by `asideLabel`                               | Inputs                                        |
| `panels`  | Up to two, each a `legend` and its `content`: the fields in the order they are filled; the skeleton wraps each in a `FieldSet`, `gap-4` | A card or a border around a panel             |
| `status`  | A state block (`ErrorState`) in place of the columns when the values could not be loaded                                                | The loading layout; draw `Skeleton`s in place |

Library density: `sm` controls. Use it when a form has more than a few fields
or holds lists of chips, so it would scroll inside a `FormDialog`. Leaving
with unsaved changes asks first with a `ConfirmDialog`.

## Steps page

Item `steps-page`. `StepsPage` is a page of its own outside the shell for
steps taken in a fixed order, one at a time, such as the first setup. A header
band `navbar` high, sticky at the top, with a bottom border, holds
`BrandHomeLink` at the left and the step strip in the middle: each step's name
after its number, the current one in `primary` with `aria-current="step"`,
done ones with a check. Below `sm` the strip is the `progress` text
(`Step 2 of 3`) and the current name only. Under the band one column, at most
`max-w-2xl` and centered, `p-4` (`p-6` from `sm`), holds the step's `title`
(the page's `h1`, `text-xl`), its `description`, the `content` `gap-4` apart,
the `notice` and the footer row.

| Slot      | Put in it                                                                                                     | Never                                          |
| --------- | ------------------------------------------------------------------------------------------------------------- | ---------------------------------------------- |
| `steps`   | Every step in order, each with its `name` and a `label` for screen readers that says done or current          | Steps that appear or vanish as the viewer goes |
| `content` | The step's own controls and lines: rows, an `outline` `sm` button that adds to them, toggles, an `Alert` note | The step's main action; it goes in the footer  |
| `notice`  | The `destructive` `Alert` of a failed save, so it sits right above the buttons that retry it                  | Field errors; they go under their field        |
| `skip`    | `Skip` (`ghost` `sm`), on every step, so skipping is never hidden                                             | A link out of the steps                        |
| `action`  | The one `default` `sm` button that finishes the step                                                          | A second `default` button                      |

While a step is saved, disable both footer buttons. Use it only when the steps
must be taken in order and the screen is nothing else; one form is a
`CenteredForm`, and settings the viewer changes in any order are a
`SettingsPage`.

## Sections

| Section        | Item            | What it is                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| -------------- | --------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PageHeader`   | `page-header`   | The page title (`h1`, `text-xl`), a count, a one-line description, `leading` (Back, breadcrumb), actions; `titleRef` makes the title focusable for when nothing else can take the focus; `titleHidden` keeps the title for screen readers only, when a breadcrumb already shows the same name                                                                                                                                                                                                                                                |
| `Toolbar`      | `toolbar`       | Search, filter triggers, then view controls and actions; `page` wraps and collapses the view controls below `lg`, `topBar` is one row in the top bar that collapses them per control; `searchPlacement="end"` puts the search after the filters at the row's end                                                                                                                                                                                                                                                                             |
| `PageSection`  | `page-section`  | A titled card (`h2`, `text-lg`); each direct child is one row, divided by a line and padded by the section                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `FormRow`      | `form-row`      | A setting: label and description on the left, one control on the right; stacked below `sm`                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `FactList`     | `fact-list`     | Term and value pairs in two columns                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `CardGrid`     | `card-grid`     | Cards at least `card-0` to `card-3` wide, stretched to fill the row, so both edges line up with the toolbar                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `GroupedList`  | `grouped-list`  | Rows under headings (`h2`, `text-sm font-semibold`, with an optional `detail` after a `·` in `text-xs` muted on the same line) that stick under the top bar while their group scrolls; each row `py-2` on the page background with no card, no line and no horizontal padding, `gap-2` between a heading and its rows, and groups `gap-6` apart                                                                                                                                                                                              |
| `CardList`     | `card-list`     | One-line cards stacked `gap-2` apart, each on `muted` with a `border` line, `rounded-lg`, `px-4`, at least `activity-card` high, in one of four tones (`default`, `primary`, `destructive`, `warning`); an icon, a title truncated first, a `badge` never truncated, a `detail` pushed right and truncated after the title, then a time column at least `activity-time` wide that never wraps and an actions column of fixed width; below `lg` two lines with a short time after the detail                                                  |
| `JumpList`     | `jump-list`     | A `ToggleGroup type="single"` of jump targets in groups divided by `Separator`s; from `lg` a vertical list of full-width, left-aligned `sm` toggles in the muted colour under an `h2` (`text-sm font-semibold`) with an optional `action` after a last divider, and below `lg` one horizontally scrolling line of outlined `sm` chips with the heading for screen readers only and no `action`                                                                                                                                               |
| `DataTable`    | `data-table`    | A `Table` on a card with its border; rows, heads and cells come from `table`; `stickyHeaderTop` sticks the column header that many px from the top, under an admin table page's stuck band                                                                                                                                                                                                                                                                                                                                                   |
| `ShelfRow`     | `shelf-row`     | Shelves `gap-6` apart in `ShelfRows`, each a heading row (`h2`, `text-base font-semibold`: the title, an optional `leading` icon, an optional `href` making both a link, `actions` at the end) above one row of cards `card-0` wide and `gap-3` apart that scrolls sideways and snaps to a card's start; from `md` with a pointer that hovers, `ghost` `icon-sm` previous and next buttons over the row's ends while it is hovered or focused, each absent at its end; or the row's `loading` skeletons or its `failed` `Alert` with `Retry` |
| `SelectionBar` | `selection-bar` | The count of selected items, `Clear selection` and the bulk actions; stuck to the bottom of the page, or in place of an admin table page's header (`placement="header"`)                                                                                                                                                                                                                                                                                                                                                                     |

- `Toolbar`: put the search in `search`, filter triggers as children, view
  controls (sort, view mode, card size) in `view`, and actions that work
  without a selection in `actions`. Each `view` entry is `{ id, label,
control }`: inline, the controls stand without a visible name, so each needs
  an `aria-label`; collapsed, the popover shows `label` above each control. A
  control renders in both places, so give it no `id`.
  - `placement="page"` (default) sits in the page, wraps, and collapses all
    view controls into a labelled `View and sort` button below `lg`.
  - `placement="topBar"` sits in the top bar through `TopBarPortal` and never
    wraps: the search takes the remaining width, and each entry joins the row
    from its `inlineFrom` (`md`, `lg` or `xl`, default `lg`). Below that width
    it moves into an icon-only `View and sort` button. An entry can pass
    `compact` for its popover form (the sort's two radio columns, with
    `compactLabelled` when that form has its own legend) and `hideBelowSm`
    when it means nothing in one column (card size).
- `GroupedList`: pass a `label`, then one `GroupedListGroup` per heading
  (`heading`, and `detail` for the muted second part, such as the date after a
  day's name) with its rows as `GroupedListItem`s. A row lays its children
  out in one line; put the row's own content in it and write no padding,
  border, card or gap around the rows. A row's first element aligns with the
  heading, so a hover fill on a pressable row reaches the heading's edge. Use it when the headings carry information the rows then leave
  out, such as the day of a history; a table with a repeated column, or cards,
  is the wrong shape for it. Example block `grouped-list-example`.
- `CardList`: pass a `label`, then one `CardListItem` per record with its
  `tone` (`primary` for work in progress, `destructive` for a failure,
  `warning` for something waiting on the owner, `default` otherwise), `icon`
  (a `size-5` status icon with its name), `title`, `badge`, `detail` (give its
  text element `truncate`), `time` and `timeShort`, and `actions` (`ghost`
  `icon-sm` buttons). Write no padding, border, surface or gap around a card.
  Use it for records that each need their state seen before they are read,
  such as an activity log; rows grouped under headings are `GroupedList`, and
  records to sort and select are `DataTable`. Example block
  `card-list-example`.
- `Toolbar` `nowrap` (`page` only): the row never wraps and scrolls sideways
  when it does not fit, for a row of filters that must stay one line. The
  search shows from `md`; below it the screen puts an `outline` `icon-sm`
  search button first among the children (`md:hidden`), which opens the field
  over the row.
- `Toolbar` `searchPlacement="end"` (`page` only): the search moves after the
  filters to the row's end, no wider than `max-w-sm`, for a row that begins
  with the page title (`ListPage toolbarRow="header"`); below `sm` it still
  takes its own line. `searchHiddenBelowLg` leaves it out below `lg` while the
  screen's own header toggle has not revealed it.
- `JumpList`: put it in `ListPage`'s `aside`. Pass the `title`, the `groups`
  of items (`{ value, label }`, newest first), the pressed `value` and
  `onValueChange`, which receives `null` when the pressed item is pressed
  again. While the items load pass `loading`; when there are none pass
  `emptyText`, shown from `lg` (below `lg` the strip is not drawn); when they
  failed pass `failed` with the message and `Retry`. `action` is the one
  quiet action that ends the column from `lg`, a `ghost-destructive` `sm`
  `Button`; below `lg` the screen offers it elsewhere, such as a `More` menu.
  The pressed chip is scrolled into view in the strip. Use it for jumping
  within one long list; a calendar would draw empty days.
- `ShelfRow`: put the shelves in `ShelfRows` in a `ListPage` body, one
  `ShelfRow` each, and pass the cards as its children; the row sets their
  width, so a card fills it. A shelf that opens a page of its own passes `href`
  and a `leading` icon; a fixed shelf passes neither. `See all` goes in
  `actions` as a `link` `sm` `Button`. While the row's items load, pass
  `loading` with its name; when they failed, pass `failed` with the message and
  `Retry`; the heading stays in both. The previous and next buttons stay out of
  the Tab order, since tabbing to a card scrolls the row. Use it for rows of
  items the viewer chooses from, not for one list to filter; the page skeleton
  is `ListPage`, its body `LoadingState layout="shelves"` while the shelves are
  unknown. Example block `shelf-row-example`.
- `PageSection`: put rows directly inside; do not add padding or borders to a
  row. Free text goes in one `<p>`, which becomes one padded row.
- `SelectionBar`: use `ghost` `sm` buttons with a visible name at every width,
  so the actions read on devices without hover; the bar wraps to a second line
  when they do not fit. A `destructive` action goes only through a
  `ConfirmDialog`.
  While a bulk action is in flight, disable the actions and pass
  `clearDisabled` so `Clear selection` is disabled too.

## States

Every list, table and section shows its data or exactly one of these blocks,
in the body slot of its skeleton, so the header and the toolbar never move.

| State            | Block                            | Shows                                                                                                                                                                        |
| ---------------- | -------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Loading          | `LoadingState` (`loading-state`) | `Skeleton` shapes of the final layout: `grid` cards, `table` rows, `grouped` rows under a heading, `cards` of a `CardList` or `shelves` of `card-0` cards under heading bars |
| Empty            | `EmptyState` (`empty-state`)     | An icon, a title saying why, a description, and the action that fills it when the viewer can take it                                                                         |
| Error            | `ErrorState` (`error-state`)     | A `destructive` `Alert`: what failed, and `Retry` when retrying can help                                                                                                     |
| Loading more     | `LoadMoreRow` (`load-more-row`)  | The content, then a `Spinner` row at the end                                                                                                                                 |
| Load more failed | `LoadMoreRow` (`load-more-row`)  | The content kept, then an `Alert` row with `Retry`, which requests the same page again                                                                                       |

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
