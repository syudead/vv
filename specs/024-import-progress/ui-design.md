# UI design: Import progress and results

**Feature**: [parent Issue #444](https://github.com/syudead/vv/issues/444) ·
[plan.md](plan.md) · [contracts/scan-api.md](contracts/scan-api.md)

Sources: the visual rules follow these documents.

- Shell: [Library UI](../../docs/design-docs/library-ui.md)
- Role tokens: [`web/src/index.css`](../../web/src/index.css)
- Contrast pairs under test: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)

The placement and interaction of the progress display stay as decided by the
[scan progress UI](../012-scan-progress/ui-design.md) (012 below):

- one indicator, fixed at the bottom right
- clicking it goes to `/settings#scan-status`
- hover and focus open the summary
- a button dismisses the failure notice
- the Settings section sits before Media folders
- a transient fetch failure keeps the last state

This document replaces **what** 012 shows. It adds no colour, radius or shadow
token.

It replaces three parts of 012:

- the text line in 012 "Floating Indicator"
- the content of 012 "Summary Popover"
- items 2 to 4 of 012 "Settings Scan Status" (the count lines and the time lines)

This document's [States](#states) replaces the 012 "States" table. When the
screen unit is implemented, 012 records that and points here (the screen unit in
[plan.md](plan.md)).

Screen text is English.

| Aspect | Rule |
| --- | --- |
| Where strings live | The string catalog [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts), as described in [Screen text and formatting (i18n)](../../docs/design-docs/i18n.md). The current `shell.scan` and `settings.scanStatus` keys are replaced with the words in this document. |
| Status of the English here | A proposal that shows the intent of each string. After implementation, the catalog is the source of truth. |
| Video counts and numbers | `videos(n)` and `formatNumber` (including `selectPlural`) separate singular and plural. |
| Dates and times | Written with `formatDateTime`. |
| User data (file and folder names) | Not translated; embedded as arguments. |

## Screen Boundary

- The bottom-right body, the summary (popover) and the Settings "Scan status" are
  built from one presentation model that reads the same `Scan` (the role of
  today's `scanPresentation.ts`). The three surfaces show the same elements in the
  same order, and a wider surface adds elements (requirement 8).

  | Element | Bottom-right body | Summary | Settings |
  | --- | --- | --- | --- |
  | Status | Yes | Yes | Yes |
  | Progress (video count) | Yes (numbers only) | Yes (bar and sentence) | Yes (bar, sentence, and what is counted) |
  | Current activity, or completion time | — | Yes | Yes |
  | Issue counts | Yes | Yes | Yes |
  | Issue list | — | — | Yes |
  | Reason the import itself failed, and retry | — | — | Yes |

- The Settings section shows no number that is not in this table. Today's
  "Processed / Total / Failed", "Preparation left" and "Started / Finished" are
  removed.
- A row of the issue list leads to the playback screen (`/videos/{id}`). The list
  sits inside the Settings section, not on a separate screen or in a dialog.

## Words

Status, progress and the current activity use these words.

**Status words**:

| `Scan.status` | Status word | Icon |
| --- | --- | --- |
| `finding` | Scanning | `RefreshCw` (spinning) |
| `running` | Scanning | `RefreshCw` (spinning) |
| `done` | Done | `CheckCircle2` |
| `partial` | Some failed | `AlertTriangle` |
| `failed` | Scan failed | `XCircle` |

From the start request until the first `Scan` arrives, the word is "Starting", as
today.

**Progress sentence**: "M of N videos done".

- The denominator covers changed files and videos whose preparation was still
  pending.
- Settled includes videos that ended in failure. Whether they are usable is
  answered by the issue counts.
- The bottom-right body writes the same value as "M / N".
- In Settings, one line under the progress sentence explains the denominator
  (requirement 9): "Counts changed files and videos that still needed preparing,
  not the whole library."
- When the import ends with `videos.total = 0`, "No changed files were found."
  replaces the progress sentence, with no bar and no numbers (acceptance
  criterion 11).
- No numbers are shown during `finding`.

**Current activity line**: "{what is happening} · {file name}". The action comes
first so that the action words remain when a long file name is truncated at the
end.

| `activity.kind` | What is happening |
| --- | --- |
| (`finding` with no activity) | Looking for files… (no file name) |
| `registering` | Adding to the library |
| `probe` | Analyzing |
| `thumbnail` | Creating the thumbnail |
| `seekThumbnail` | Creating seek thumbnails |
| `preview` | Creating the preview |

If `activity` disappears for a moment during `running`, the line keeps the
previous value instead of going blank. The line height is fixed at one line
(acceptance criterion 14).

**Completion time line**: after the import ends, the time takes the place of the
current activity line.

| Status | Line |
| --- | --- |
| `done` | "Finished {date and time}" |
| `partial` | "Finished {date and time}". The status word is already "Some failed", so the time line does not distinguish them. |
| `failed` | "The scan couldn't finish." and no time. A `failed` `Scan` has no time ([contracts/scan-api.md §2](contracts/scan-api.md#2-scan)); the failure reason and retry tell when the result is from. |

The time is `Scan.settledAt` written with `formatDateTime` (for example, Sep 28,
2026, 3:04 PM). No time is shown while running (requirement 4).

**Issue counts**:

- Failures read "N failed" with `AlertTriangle`.
- Substitutions read "N to check" with `Info`.
- A count of 0 is not shown.
- The bottom-right body shows only failures when there are any, and otherwise the
  to-check count.

**Issue impact and reason**: each row of the list shows the impact of the main
kind as the first sentence and the reason as the second. The main kind is the
first entry of `kinds`
([contracts/scan-api.md §3](contracts/scan-api.md#3-get-apiscanscurrentissues)).
The impact says, in the user's words, what state the video is in
(requirement 5).

| `kind` | Impact | Reason |
| --- | --- | --- |
| `unreadable` | Not added to the library. | The file couldn't be read. |
| `changed_during_import` | Not added to the library. | The file changed during the scan. |
| `register_failed` | Not added to the library. | Saving it to the library failed. |
| `probe_failed` | It may not play. | It couldn't be analyzed as a video. |
| `thumbnail_failed` | It has no thumbnail. | The thumbnail couldn't be created. |
| `seek_thumbnail_failed` | No thumbnails appear while seeking. | The seek thumbnails couldn't be created. |
| `preview_failed` | No preview appears in the list. | The preview couldn't be created. |
| `thumbnail_first_frame` | The thumbnail uses the first frame instead. | The frame at the usual position couldn't be read. |
| `seek_thumbnail_full_decode` | The seek thumbnails were rebuilt from the whole video. | The usual method didn't work. |

The second and later kinds are combined, impact only, into one line:
"Also: {impact} {impact}". The catalog holds this table as a `Record` keyed by
`kind`, so a missing kind is caught by the type check.

## Floating Indicator

Same position and surface as 012 (`bg-elevated`, `border-border-strong`,
`shadow-elevated`, `rounded-md`). The body is one line with, from the left:

1. the icon
2. the status word
3. "M / N"
4. the issue counts

- The status word is `font-medium` `text-fg`, stronger than the other elements.
  Numbers are `tabular-nums`. Issue counts use the same size and colour
  (`text-fg`) as the progress and are told apart by icon. Only the icons take a
  semantic colour: `text-danger` for failures, `text-warning` for to-check.
- The current activity is not shown in the body, so the body's width does not
  change with each activity. The right edge is fixed, so it does not move when the
  number of digits changes.
- **How long it stays visible**:

  | Status | Visibility |
  | --- | --- |
  | `finding`, `running` | Shown throughout |
  | `done` | Closes after 8 seconds, like 012's completion; also when there are only substitutions |
  | `partial` | Stays, like `failed`, until dismissed with the button or until the owner goes to Settings, so that an owner who was away still sees that some videos are unusable. The dismiss button's name is "Dismiss the scan result notice". |

- Click, Enter and Space behave as in 012 and go to `/settings#scan-status`.

## Summary Popover

Opens and closes as in 012. The content, from the top, is three lines and a bar:

1. Status line: the icon and the status word (`font-semibold`), with the issue
   counts at the right end. When there are both failures and to-check items, both
   appear.
2. The bar (the same `ScanProgressBar` as 012) and the progress sentence below it.
3. The current activity line, or the completion time line.
   - `text-sm` `text-fg-muted` on one line; a long file name is truncated at the
     end.
   - The full truncated value (registered folder display name / relative path /
     file name) is readable through `title`.

For `partial`, and for `done` with to-check items, "See Settings for the list." is
added below line 3 in `text-xs`. It is the same destination as clicking the body,
so it is not a link.

The per-job-kind breakdown (`ProcessingBreakdown`) is not shown.

## Settings Scan Status

The section frame, heading, and focus movement to the heading stay as in 012. The
content, from the top:

1. **Heading row**: "Scan status" and the status badge (icon and status word).
   The issue counts follow the badge as badges of the same shape.
2. **Progress**: the bar, the progress sentence, and the line explaining the
   denominator.
3. **Current activity line or completion time line**: the same line as in the
   summary, at a fixed height.
4. **Failure of the import itself** (only for `failed`): as in 012, the reason in
   `text-danger` and a primary "Retry". The reason text is written with
   `scanErrorText(errorCode, errorPath)` as decided in 023. The reason sits above
   the issue list, so that the failure of the import itself is read before the
   partial issues.
5. **Issue list** (only when there is at least one issue).

Items 2 and 3 have the same order and form as the summary. Settings adds only the
denominator line, item 4 and item 5.

### Issue List

- **Heading**: "Videos with problems" (`h3`, `text-sm font-semibold`), with the
  failure and to-check counts to its right. A divider (`border-border`) separates
  the list from items 1 to 3.
- **Order**: the API order (failures first; within a severity, by file name).
- **Row**: a marker column on the left and a text column on the right.
  - Marker: an icon and a word. Failures are `AlertTriangle` and "Failed" in
    `text-danger`; to-check items are `Info` and "Check" in `text-warning`. Colour
    is never the only cue (Issue: do not rely on colour alone).
  - Text column, four lines:
    1. File name: `text-sm font-medium text-fg`, one line, truncated at the end.
    2. Location: "registered folder display name / relative path" on one
       `text-xs text-fg-muted` line, keeping the end (nearest the file) when
       truncated. This is the same truncation as the registered path on folder
       cards (library-ui.md §6).
    3. Impact and reason: `text-sm text-fg`, reason after impact, wrapping
       allowed.
    4. Other kinds: the "Also: …" line, only when there are two or more kinds.
  - The full truncated file name and location are readable through `title`. Files
    with the same name are told apart by the location line (Edge Cases). Every
    row in the list is inside a registered folder
    ([contracts/scan-api.md §3](contracts/scan-api.md#3-get-apiscanscurrentissues)),
    so the location line is never empty.
- **Going to the video**:

  | Row | Behaviour |
  | --- | --- |
  | Registered video (has `videoId`) | The whole row links to `/videos/{id}`, with `ChevronRight` at the right end, `hover-wash` on hover, and the same `link` outline as 012 on focus-visible. The link name includes the file name and the impact. |
  | Unregistered file | Not a link; nothing at the right end. |

- **Long lists**: the list is a vertically scrolling region with a maximum height
  (about half the screen height).
  - The region shows the first 50 items.
  - A "Show more" button at the end adds the next 50.
  - The position of the Media folders section below the region does not change
    however many items there are (Issue: do not push content away).
  - The region is reachable by keyboard (through the row links; when only rows
    without links follow, the region itself gets `tabIndex=0` and the name
    "Videos with problems").
  - Rejected: showing the first few items and expanding with "Show all", because
    expanding thousands of items pushes the Media folders section away.
- **Replacement**: when a new import changes `Scan.id`, the list reloads and
  returns to the top. When `issues.revision` changes within the same import, the
  list reloads and keeps the region's scroll position.

## States

| State | Bottom-right body | Summary | Settings |
| --- | --- | --- | --- |
| Never run | Not shown | None | "No scan has run yet". No bar, numbers or list |
| Starting | "Starting" | Bar without numbers, "Looking for files…" | Same |
| `finding` | "Scanning" | Bar without numbers, "Looking for files…" | Same |
| `running` | "Scanning M / N" (plus issue counts) | Bar, "M of N videos done", current activity | Same, plus the denominator line, plus the list (if there are issues) |
| `done` (N ≥ 1) | "Done N / N" for 8 seconds (plus the to-check count) | Full bar, "N of N videos done", "Finished {date and time}" | Same, plus the list (if there are to-check items) |
| `done` (N = 0) | "Done" for 8 seconds | No bar, "No changed files were found.", "Finished {date and time}" | Same |
| `partial` | "Some failed · K failed" until dismissed | Full bar, the settled sentence, "Finished {date and time}", pointer to Settings | Same, plus the list |
| `failed` | "Scan failed" until dismissed | The bar and sentence as of that moment, "The scan couldn't finish." | Same, plus reason and retry, plus the list (if any) |
| Transient fetch failure | As in 012 | As in 012 | As in 012. The list keeps the last one read |

`running` includes the period after the scan closes while only preparation
remains. That period gets no separate status word or separate numbers
(requirements 1 and 2).

## Responsive Layout

The widths judged are 360px, 768px and 1280px.

| Width | Part | Layout |
| --- | --- | --- |
| 360px | Bottom-right body | One line. When it does not fit with the issue counts, keep "M / N" and reduce the issue counts to icon and number (the words stay in the accessible name). |
| 360px | Summary | The same width as 012. The current activity line is truncated on one line. |
| 360px | Settings list row | The marker column moves to the start of the file name line as one column. The text column uses the full screen width. |
| 768px and wider | Settings list row | List rows have two columns, marker and text. The marker column is as wide as the longer of "Failed" and "Check", so the text column starts at the same position in every row. |
| 1280px | Settings section | The Settings section keeps its current maximum width (inside `max-w-4xl`). The list's text column wraps at the section width. |

At every width, these three keep their position and height:

- the bottom-right body
- the current activity line in the summary
- the current activity line in Settings

## Accessibility

Following the Issue's assistive-technology item, 012's Accessibility changes as
follows.

- `role="status"` announces only these three milestones; a change in the current
  activity or in progress is not announced.

  | Milestone | Announcement |
  | --- | --- |
  | Done | "The scan is complete." followed by "N videos are worth checking." when there are to-check items |
  | Partial failure | "The scan finished with some failures. N videos may not be usable." |
  | Failure | "The scan failed." |

- The bar's name is "Progress of the videos in this scan", with `aria-valuetext`
  "M of N videos done". During `finding` it has no `aria-valuenow` (as in 012).
- The bottom-right body's name is "{status word} M of N videos done, {issue
  counts}. Open the scan status".
- The issue list is a `ul`, and each row's link is reachable with Tab.

## Review Criteria

At each of 360px, 768px and 1280px, judge by looking:

1. **Hierarchy**: on every surface, the status word reads strongest, then the
   progress and issue counts, and the current activity line last. Internal stage
   names (scan, preparation, job kinds; today's "Preparing", "Preparation left"
   and similar) do not appear as status words or number labels.
2. **One progress value**: throughout a 10-video import, the numbers at the bottom
   right and in Settings stay "/ 10" and increase. They never switch to numbers in
   a different unit. After registration ends, the display does not show "done" or
   a full bar.
3. **Never looks stalled**: during an import, the current activity line in the
   summary and in Settings moves on with changing file names and action words.
   Meanwhile the bottom-right body, the summary line and the Settings line do not
   move or change height.
4. **Usable, and what to check**: after the import, one glance at the bottom right
   shows "done" or "some failed" and the failure and to-check counts. In Settings,
   the list has as many rows as those counts.
5. **List readability**:
   - Failure rows come before to-check rows.
   - The marker words and icons tell failures and to-check items apart without
     colour.
   - Rows hold their shape with long file names, and files with the same name are
     told apart by the location line.
   - A registered video's row leads to the playback screen.
6. **Does not push content away**: with hundreds of issues, the position of the
   Media folders section in Settings does not change. "Show more" reaches the last
   row.
7. **Density**: the Settings "Scan status" looks like the summary's elements plus
   only the denominator line, the failure reason and the list. No column of numbers
   that the summary lacks.
8. **No overlap**: none of these combinations overflows or overlaps:
   - the bottom-right body and the summary
   - Toast
   - the playback screen controls
   - long file names
