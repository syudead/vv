# UI design: Activity

Sources: [design-system.md](../../docs/design-docs/design-system.md) and its
rules ([patterns.md](../../web/registry/rules/patterns.md),
[components.md](../../web/registry/rules/components.md),
[foundations.md](../../web/registry/rules/foundations.md)); the reference image
the requester gave when the parent Issue was written (an activity screen of a
file uploader: one bordered card per run, a status icon, the file name with a
source chip, the result and the time at the right end, a live progress bar on
the running card, a red card for a failure, and a count band at the bottom);
the data and the operations are [plan.md](plan.md) and
[contracts/screen-api.md](contracts/screen-api.md).

## Why this shape

Activity is a list of cards, one per entry, each a single line on a wide
screen, as the requester's reference image draws it: the status is an icon at
the start, the problem is a coloured card the eye finds before reading, and
the result sits at the right where the eye ends the line. Every filter is one
of a row of identical dropdown buttons, the status included, so no filter
looks more important than another and the toolbar stays one row.

| Rejected | Why |
| --- | --- |
| History's day-grouped text rows with no border | Rows without a border run together; the requester rejected it on sight |
| Status as underlined tabs beside dropdown filters | Two forms for the same act of narrowing the list; the requester asked for one |
| A second toolbar row for the other filters | The requester asked for one row |
| The library's single `FilterMenu` popover | Each dimension's current value has to show on the toolbar without opening anything |
| "Pause all" and "Retry failed" in the header, as the reference has | The parent Issue leaves bulk actions out |

## Words

Screen text as `web/src/i18n/en.ts` will hold it, under `t.activity` unless
noted. `{name}` and the like are the arguments of the catalog function.

| Place | Text | Notes |
| --- | --- | --- |
| Sidebar entry (`t.shell.nav.activity`) | `Activity` | |
| Sidebar count, read aloud | `1 needs attention`, `{count} need attention` | `selectPlural`; the visible text is the number |
| Page and document title | `Activity` | |
| Search | `Search file names` | Placeholder and label |
| Filter buttons | `Status`, `Feature`, `Decided by`, `Period` | Each followed by its current value |
| Status values | `All`, `Running`, `Needs attention`, `Done` | Each but `All` with its count in the menu |
| Feature values | `All`, `Import`, `Scan`, `Duplicate check`, `Auto-tagging`, `Work info`, `Tags`, `Versions`, `Media folders`, `Drag and drop` | Several may be checked; the button shows one name, or `{first} +{n}` |
| Decided by values | `All`, `AI`, `Rule`, `You` | |
| Period values | `Any time`, `Today`, `Last 7 days`, `Last 30 days` | |
| Clear | `Clear filters` | After the buttons while any filter is set |
| Status icon, read aloud | `Running`, `Waiting`, `Needs attention`, `Done`, `Done by you` | |
| Chip: kind | `Import`, `Scan`, `Duplicate check`, `Auto-tagging`, `Work info fetch`, `Bundled`, `Removed from versions`, `Made representative`, `Marked different`, `Tag added`, `Tag removed`, `Tag kept`, `Tag rejected`, `Work info applied`, `Media folder removed`, `Drag and drop undone`, `Undo` | One per kind, in data-model order |
| Result: import running | `{stage}` with the bar and `{done}/{total}` | `{stage}`: `Analyzing`, `Thumbnail`, `Seek thumbnails`, `Preview`, `Duplicate fingerprint` |
| Result: waiting | `Waiting` | |
| Result: import failed | `Failed at {stage}: {detail}` | `{stage}` lower case |
| Result: other failure | `Failed: {detail}` | |
| Result: duplicate check failed | `Failed · tries again at {time}` | `{time}`: `14:05` today, else `Oct 17, 14:05` |
| Result: interrupted | `Stopped when vv shut down` | |
| Result: no connection | `Needs the AI connection to decide` | |
| Result: undecided | `Couldn't tell whether these are the same video` | With `AI {percent}` when the model answered |
| Result: same | `Same video · bundled by AI {percent}`, `Same video · bundled by rule` | |
| Result: different | `Different videos by AI {percent}` | |
| Result: auto-tagging | `+ {name} {percent}`, joined by ` · `; `(suggested)` after a suggested tag; `No tags added` | |
| Result: owner tag operation | `+ {name}` or `− {name}` · `by you` | |
| Result: media folder removed | `1 video left the library · by you`, `{count} videos left the library · by you` | |
| Result: superseded | `Replaced by a newer run` | |
| Result: decided by owner | `You decided on Duplicates` | |
| Result: gone | `A video left the library` | |
| Result: undo | `Undid {chip} · {result}`, `Undid tag {name}` | From `undoes` |
| Result: already reverted | `Already changed back by you` | |
| Undone suffix | `· Undone by you` | After the result of an undone entry |
| Not in the library | `Not in the library` | Replaces the result's end when the subject has no video |
| Time | `Just now`, `{n} minutes ago`, `{n} hours ago`, `Yesterday {time}`, `{date} {time}` | The full date and time in the tooltip |
| Icon buttons (tooltip and read aloud) | `Retry`, `Undo`, `Undo tags…`, `Open`, `Open in Duplicates`, `Work info`, `Settings` | |
| Footer | `{n} running`, `{n} waiting`, `{n} need attention`, `{n} done` | Running counts entries being processed; waiting those queued |
| Toast after undo | `Undone. vv won't decide this again on its own` | |
| Toast after an already reverted undo | `Already changed back. vv won't decide this again on its own` | |
| Toast on a failed retry or undo | `Couldn't retry: {reason}`, `Couldn't undo: {reason}` | `{reason}` from the error catalog |
| Error `activity_not_retryable` (error catalog) | `This can no longer be retried` | |
| Error `activity_not_undoable` (error catalog) | `This can no longer be undone` | |
| Empty, no entries at all | `No activity yet` / `Imports, auto-tagging, duplicate checks and your changes to tags and versions show up here.` | Title / description |
| Empty, filters match nothing | `Nothing matches these filters` / `No entries match these filters.` / `Clear filters` | Title / description / action |
| Load failed | `Couldn't load the activity` | With `Retry` |

## Sidebar entry

`Activity` is the last entry of the main navigation, after `Duplicates`,
owner only, with the lucide `Activity` icon. While one or more entries need
attention, the rail item carries a round `destructive` count at its top-right
corner ([Changes to the design system](#changes-to-the-design-system));
running entries add no mark. The count refreshes on the `activity`
notification on every screen.

## Activity page

`ListPage` with the title `Activity` in `PageHeader`, then one `Toolbar` row,
then the cards, then the footer band.

### Toolbar

One row, left to right: the search field, then four filter buttons of the same
form, `Status`, `Feature`, `Decided by`, `Period`, then `Clear filters` while
any is set.

| Part | Form |
| --- | --- |
| Search | `Input` with a search icon, `sm`; filters as the owner types, after a pause |
| Filter button | `outline` `sm` `Button` holding the dimension's name and its current value in `text-muted-foreground`, and a chevron; set to anything but its default, it turns `primary-soft` with `primary` text, value included |
| Status menu | `DropdownMenu` with radio items, each with its count at the right end in `tabular-nums`; the `Needs attention` count is `destructive` when above 0 |
| Feature menu | `DropdownMenu` with checkbox items and their counts, the owner-operation features after a separator |
| Decided by and Period menus | `DropdownMenu` with radio items |
| Clear filters | `ghost` `sm` `Button` |

Every filter is in the page URL (`status`, `feature`, `decider`, `period`,
`q`), pushed to history, so back returns to the previous filters.

### Cards

The cards stack with `gap-2`. Each is one row on a `muted` surface with a
`border` line and `rounded-lg` corners, the named step `activity-card` high at least, `px-4`.

| Part | Content | Form |
| --- | --- | --- |
| Status icon | `LoaderCircle` spinning (running), `Clock` (waiting), `CircleAlert` (needs attention), `CircleCheck` (done), `User` (done by you) | `size-5`, `primary`, `muted-foreground`, `destructive` or `warning` by reason, `success`, `muted-foreground` |
| Subject | The file name; a pair as `{a} ↔ {b}`; a scan as `Manual scan` or `Watch scan`; a folder as its path. An item with a video links to the video page | `text-sm font-medium`, one line, truncated first when the card runs out of width |
| Chip | The kind | `Badge` `secondary`, never truncated |
| Result | The result text, the AI percentage in `foreground` | `text-sm text-muted-foreground`, right-aligned, one line, truncated before the chip is |
| Progress | For a running import: the stage, a bar the named step `activity-bar` wide, and `{done}/{total}` in `primary` | `Progress` |
| Time | Relative time | `text-sm text-muted-foreground`, the named step `activity-time` wide, right-aligned |
| Actions | Icon buttons the entry offers | `ghost` `icon-sm`, in a fixed-width end column so the times line up |

| Card state | Border and surface |
| --- | --- |
| Running (processing) | `primary` border, `primary-soft` surface |
| Needs attention: `failed`, `interrupted` | `destructive-strong` border, `destructive-soft` surface; the result in `destructive` |
| Needs attention: `no_connection`, `undecided` | `warning` border, `warning-soft` surface; the result in `warning` |
| Everything else | `border`, `muted` |

| Action | Icon | Shown when | After |
| --- | --- | --- | --- |
| Retry | `RotateCw` | `retryRun` | A new running card at the top and this one reads `Replaced by a newer run`; a duplicate check's card turns running instead; a scan's card stays until a scan ends without failing |
| Undo | `Undo2` | `undoable` and not `auto_tag` | Toast; the card adds `· Undone by you`; a new card by you at the top |
| Undo tags… | `Undo2` | An `auto_tag` with a tag not undone | Opens a `DropdownMenu` of the added tags with their percentages; choosing one undoes that tag, with the same toast |
| Open | `ExternalLink` | A video subject in the library | The video page |
| Open in Duplicates | `ExternalLink` | A pair whose `candidate` is true | `/duplicates` |
| Work info | `FileText` | A `work_info_fetch` whose video is in the library | `/videos/{id}/edit` |
| Settings | `Settings` | Reason `no_connection` | `/settings` |

While a request is pending its button shows a `Spinner` and every button of
the card is disabled. Neither retry nor undo asks for confirmation: an undo
returns the state from before an automatic decision, which the owner can make
again by hand. A failed request leaves the card as it was and shows the toast.

The cards are newest first by `createdAt`, loaded page by page as the owner
scrolls, with `LoadMoreRow` at the end.

### Footer band

A band under the cards with a top `border`, `activity-card` high: `{n} running`,
`{n} waiting`, `{n} need attention` from the left, each number in `font-semibold`
(`primary`, `foreground`, `destructive`), and `{n} done` at the right end. It
counts the whole log whatever the filters, as the sidebar does, so the owner
sees work in progress and problems while looking at anything else.

### Live updates

On the `activity` notification the page refetches the counts and every page it
has loaded and replaces the cards in place. A card that no longer matches the
filters leaves; new cards join the top. A running import's bar moves on each
notification, since each finished stage sends one.

### States

| State | What the page shows |
| --- | --- |
| Loading | `LoadingState` under the toolbar |
| Load failed | `ErrorState` with `Couldn't load the activity` and `Retry` |
| No entries at all | `EmptyState` with the `Activity` icon, `No activity yet` and its description |
| Filters match nothing | `EmptyState` with the `ListFilter` icon, `Nothing matches these filters`, its description and `Clear filters` |
| Loading more, load more failed | `LoadMoreRow` as every list |

Guests have no sidebar entry, and `/activity` sends them to sign in, as every
owner-only screen does ([016 UI design](../016-single-account-auth/ui-design.md)).

## Changes to the design system

| Addition | What it is | Why |
| --- | --- | --- |
| Filter dropdown button | An `outline` `sm` `Button` with the dimension's name, its value and a chevron, opening a `DropdownMenu`; `primary-soft` when not at its default. Goes in components.md beside `DropdownMenu`, with a showcase entry | Activity filters on four dimensions that each need their value visible; the library's `FilterMenu` hides them behind one button |
| Card list (`CardList` section) | A stack of one-line bordered cards on `muted`, `gap-2`, with four tones (default, `primary`, `destructive`, `warning`) set by the caller and fixed columns at the end for the time and the actions. Goes in patterns.md with an example block | No section draws bordered rows; `GroupedList` rows have no border and `DataTable` has a header and columns that do not fit one line at 360px |
| Rail count | A rail entry takes a count, drawn as a round `destructive` pill at the icon's top-right corner, `99+` above 99, nothing at 0; in the narrow-width drawer, `SidebarMenuBadge` at the end of the row. Goes in components.md, Sidebar | `SidebarMenuBadge` is hidden when the sidebar is collapsed to icons, which vv's rail always is |
| Named sizes `activity-card`, `activity-time`, `activity-bar` | The card's minimum height (52px), the time column (104px) and the progress bar (160px) | The 4px scale stops at 64px and has no 52px step |

## Responsive behaviour

| Width | Layout |
| --- | --- |
| 360px | The toolbar is one row that scrolls sideways: a search icon button that opens the field over the row, then the filter buttons. Each card is two lines: the status icon and the subject, with the action buttons at the right spanning both lines; under the subject, the chip, the result (truncated) and the short time (`15m`). The footer shows the three counts, without `done` |
| 768px | As 360px, with the search field in place of the icon button; the four filter buttons fit |
| 1280px | One toolbar row with the search field and the four buttons. Each card is one line: icon, subject, chip, the result pushed to the right, time, actions |

## Review criteria

1. At 1280px each card is one line with the result, the time and the actions
   lined up in columns down the list; at 360px each card is two lines and no
   card is wider than the screen.
2. A failed card stands out in red and a waiting-for-AI card in amber before
   any text is read, and a running import shows its bar moving.
3. A 120-character file name is cut with an ellipsis before the chip or the
   result is; the status icon, the time and the actions are never cut.
4. All five filter controls share one height, border and type; a set filter
   reads its value on its button without opening it.
5. The counts in the Status menu equal the cards each choice lists, and the
   footer and the sidebar count equal the whole log's counts whatever the
   filters.
6. A card offers only the buttons its entry allows: an import that finished
   offers Open only; a failed import offers Retry; an automatic bundle offers
   Undo and Open; an undone entry offers no Undo.
7. With the page open, a finished import turns its card from running to done
   without a reload, and the card the owner was reading does not move.
8. The gaps between cards and the page margins match the `ListPage` and
   `CardList` steps at every width.
