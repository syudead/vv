# UI Design: import progress and results

**Feature**: [parent Issue #444](https://github.com/syudead/vv/issues/444) ·
[plan.md](plan.md) · [contracts/scan-api.md](contracts/scan-api.md)

Visual rules follow these documents:
- Shell: [Library UI](../../docs/design-docs/library-ui.md)
- Role tokens: [`web/src/index.css`](../../web/src/index.css)
- Pairs checked for contrast: [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts)

The placement and interaction of the progress display reuse the shape decided by
[Video import progress UI](../012-scan-progress/ui-design.md) (012 below) unchanged:
- One fixed element at the bottom right.
- Clicking it goes to `/settings#scan-status`.
- Hover and focus open the summary.
- A button dismisses the failure notice.
- The settings section sits before the media folders.
- A temporary fetch failure keeps the last state.

This document replaces **what 012 shows**. It adds no new color, radius or shadow tokens.

It replaces three parts:
- The text line of 012 "Floating Indicator"
- The content of 012 "Summary Popover"
- Items 2 to 4 of 012 "Settings Scan Status" (the count lines and the time lines)

[States](#states) in this document replaces the 012 "States" table. When the screen units land,
012 records this and points here (the screen units in [plan.md](plan.md)).

The screen text is English.
- Location: the string catalog [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts), as described in
  [Screen text and formatting (i18n)](../../docs/design-docs/i18n.md). The current `shell.scan`
  and `settings.scanStatus` keys are replaced with the text in this document.
- Status of the English here: a proposal that shows the intent of the text. After
  implementation, the catalog is canonical.
- Counts: `videos(n)` and `formatNumber` (including `selectPlural`) distinguish singular and
  plural.
- Dates and times: written with `formatDateTime`.
- User data (file names, folder names): embedded as arguments, untranslated.
- Prose: design documents were left out of the UI translation (023
  [research.md R-8](../023-english-i18n/research.md)).

## Screen Boundary

- The bottom-right indicator, the summary (popover) and the Settings "Scan status" section are
  built from one display model that reads the same `Scan` (today the role of
  `scanPresentation.ts`). The three surfaces list the same elements in the same order and add
  elements as the surface grows (Requirement 8).

  | Element | Bottom-right indicator | Summary | Settings |
  | --- | --- | --- | --- |
  | State | Yes | Yes | Yes |
  | Progress (video count) | Yes (numbers only) | Yes (bar and sentence) | Yes (bar, sentence and target explanation) |
  | Current activity, or completion time | — | Yes | Yes |
  | Issue counts | Yes | Yes | Yes |
  | Issue list | — | — | Yes |
  | Failure reason of the import itself, and retry | — | — | Yes |

- The Settings section shows no number outside this table. The current "Processed / Total /
  Failed", "Preparation left" and "Started / Finished" are removed.
- A row in the issue list goes to the player (`/videos/{id}`). The list lives inside the Settings
  section, not on a separate screen or in a dialog.

## Words

The state, progress and current activity use the text below.

**State text**:

| `Scan.status` | State text | Icon |
| --- | --- | --- |
| `finding` | Scanning | `RefreshCw` (spinning) |
| `running` | Scanning | `RefreshCw` (spinning) |
| `done` | Done | `CheckCircle2` |
| `partial` | Some failed | `AlertTriangle` |
| `failed` | Scan failed | `XCircle` |

Between the start request and the first `Scan`, the text is "Starting", as today.

**Progress sentence**: "M of N videos done".
- The denominator includes changed files and videos whose preparation remained.
- "Done" includes videos that ended in failure. The issue counts answer whether they are usable.
- The bottom-right indicator writes the same values as "M / N".
- Settings adds one line under the progress sentence that explains the denominator
  (Requirement 9): "Counts changed files and videos that still needed preparing, not the whole
  library."
- When the import ends with `videos.total = 0`, "No changed files were found." replaces the
  progress sentence, with no bar and no numbers (acceptance criterion 11).
- No numbers appear during `finding`.

**Current activity line**: "〈action〉 · 〈file name〉". The action comes first so that the action
text survives when a long file name is truncated at the end.

| `activity.kind` | Action |
| --- | --- |
| (`finding` with no activity) | Looking for files… (no file name) |
| `registering` | Adding to the library |
| `probe` | Analyzing |
| `thumbnail` | Creating the thumbnail |
| `seekThumbnail` | Creating seek thumbnails |
| `preview` | Creating the preview |

If `activity` disappears briefly during `running`, the line keeps the previous value instead of
going blank. The line height is fixed at one line (acceptance criterion 14).

**Completion time line**: after the import ends, the time takes the position of the current
activity line.
- `done`: "Finished 〈date and time〉"
- `partial`: "Finished 〈date and time〉". The state text already says "Some failed", so the time
  line does not distinguish.

The date and time are `Scan.settledAt` written with `formatDateTime` (example: Sep 28, 2026, 3:04
PM). No time appears while running (Requirement 4). `failed` shows no time; the same position
shows "The scan couldn't finish." A `failed` `Scan` has no time
([contracts/scan-api.md §2](contracts/scan-api.md#2-scan)). The failure reason and retry tell
which run the result belongs to.

**Issue counts**:
- Failures read "N failed" with `AlertTriangle`.
- Substitutions read "N to check" with `Info`.
- A count of 0 is not shown.
- The bottom-right indicator shows only the failures if any exist, otherwise the to-check count.

**Issue impact and reason**: each list row shows the impact of the primary kind as the first
sentence and the reason as the second. The primary kind is the first of `kinds`
([contracts/scan-api.md §3](contracts/scan-api.md#3-get-apiscanscurrentissues)). The impact says,
in the user's words, "what state the video is in" (Requirement 5).

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

The second and later kinds show only their impact, combined into one line: "Also: 〈impact〉
〈impact〉". The catalog holds this table as a `Record` keyed by `kind`, so type checking catches a
missing kind.

## Floating Indicator

Same position and same surface as 012 (`bg-elevated`, `border-border-strong`, `shadow-elevated`,
`rounded-md`). The body is one line, left to right:
1. Icon
2. State text
3. "M / N"
4. Issue counts

- The state text is `font-medium` `text-fg`, stronger than the other elements. Numbers use
  `tabular-nums`. The issue counts use the same text size and color as the progress (`text-fg`)
  and are told apart by icon. Only the icon carries a semantic color: `text-danger` for failures,
  `text-warning` for to-check.
- The current activity is not shown in the body, so the body width does not change with each
  activity. The right edge is fixed, so it does not move when the number of digits changes.
- **Visible period**:
  - Shown throughout `finding` and `running`.
  - `done` closes after 8 s, like the 012 completion. The same applies with substitutions only.
  - `partial`, like `failed`, stays until dismissed with the button or until the user goes to
    Settings. This shows unusable videos to an owner who was away. The dismiss button is named
    "Dismiss the scan result notice".
- Click, Enter and Space behave as in 012 and go to `/settings#scan-status`.

## Summary Popover

Opens and closes as in 012. The content is three lines and a bar, top to bottom:

1. State line: icon and state text (`font-semibold`). Issue counts at the right end. When both
   failures and to-check exist, both appear.
2. The bar (the same `ScanProgressBar` as 012) with the progress sentence below it.
3. The current activity line, or the completion time line.
   - One line in `text-sm` `text-fg-muted`; a long file name is truncated at the end.
   - The full truncated text (registered folder display name / relative path / file name) is
     readable through `title`.

For `partial`, and for `done` with to-check items, "See Settings for the list." appears under
line 3 in `text-xs`. It goes to the same place as clicking the body, so it is not a link.

The per-job-kind breakdown (`ProcessingBreakdown`) is not shown.

## Settings Scan Status

The 012 section frame, heading and focus move to the heading do not change. The content, top to
bottom:

1. **Heading line**: "Scan status" and the state badge (icon + state text). The issue counts follow
   the badge as badges of the same shape.
2. **Progress**: the bar, the progress sentence and the denominator line.
3. **Current activity line, or completion time line**: the same line as the summary. Fixed height.
4. **Failure of the import itself** (only for `failed`): as in 012, the reason in `text-danger`
   and the primary "Retry" action. The reason text uses `scanErrorText(errorCode, errorPath)` as
   decided by 023. The reason sits above the issue list, so the failure of the import itself is
   read before partial issues.
5. **Issue list** (only when at least one issue exists).

Items 2 and 3 have the same order and shape as the summary. Settings adds only the denominator
line, item 4 and item 5.

### Issue List

- **Heading**: "Videos with problems" (`h3`, `text-sm font-semibold`), with the failure and
  to-check counts to its right. A divider (`border-border`) separates the list from items 1 to 3.
- **Order**: the API order as is (failures first, then by file name within the same severity).
- **Row**: a marker column on the left, a text column on the right.
  - Marker: icon and word. Failures show `AlertTriangle` and "Failed" in `text-danger`; to-check
    shows `Info` and "Check" in `text-warning`. Color is never the only difference (Issue: "do not
    rely on color alone").
  - Text column, four lines:
    1. File name: `text-sm font-medium text-fg`, one line, truncated at the end.
    2. Location: "registered folder display name / relative path" in one `text-xs text-fg-muted`
       line, keeping the end (the side nearest the file). This matches the truncation of the
       registered path on folder cards (library-ui.md §6).
    3. Impact and reason: `text-sm text-fg`. The reason follows the impact; wrapping is allowed.
    4. Other kinds: the "Also: …" line, only when there are two or more kinds.
  - The full truncated file name and location are readable through `title`. Files with the same
    name are told apart by the location line (Edge cases). Every listed row lies inside a
    registered folder ([contracts/scan-api.md §3](contracts/scan-api.md#3-get-apiscanscurrentissues)),
    so the location line is never empty.
- **Go to the video**:
  - Row of a registered video (has `videoId`): the whole row links to `/videos/{id}`.
    `ChevronRight` sits at the right end, hover applies `hover-wash`, and focus-visible shows the
    same `link` outline as 012. The link name includes the file name and the impact.
  - Row of an unregistered file: not a link, nothing at the right end.
- **Long list**: the list is a vertical scroll region with a height cap (about half the screen
  height).
  - The region shows the first 50 entries.
  - A "Show more" button at the end adds the next 50.
  - The "Media folders" section below the region stays in place however many entries the list
    has (Issue: "do not push content down").
  - The region is keyboard-reachable (through the row links; when only rows without links remain,
    the region itself gets `tabIndex=0` and the name "Videos with problems").
  - Rejected: showing the first few entries and expanding with "show all". Expanding pushes the
    "Media folders" section down by thousands of entries.
- **Replacement**: when a new import changes `Scan.id`, the list reloads and returns to the top.
  When `issues.revision` changes within the same import, the list reloads and keeps the region's
  scroll position.

## States

| State | Bottom-right indicator | Summary | Settings |
| --- | --- | --- | --- |
| Never run | Hidden | None | "No scan has run yet". No bar, numbers or list |
| Starting | "Starting" | Bar without numbers, "Looking for files…" | Same |
| `finding` | "Scanning" | Bar without numbers, "Looking for files…" | Same |
| `running` | "Scanning M / N" (+ issue counts) | Bar, "M of N videos done", current activity | Same + denominator line + list (if issues exist) |
| `done` (N ≥ 1) | "Done N / N" for 8 s (+ to-check count) | Full bar, "N of N videos done", "Finished 〈date and time〉" | Same + list (if to-check items exist) |
| `done` (N = 0) | "Done" for 8 s | No bar, "No changed files were found.", "Finished 〈date and time〉" | Same |
| `partial` | "Some failed · K failed" until dismissed | Full bar, done sentence, "Finished 〈date and time〉", pointer to Settings | Same + list |
| `failed` | "Scan failed" until dismissed | Bar and sentence at that point, "The scan couldn't finish." | Same + reason and retry + list (if any) |
| Temporary fetch failure | Same as 012 | Same as 012 | Same as 012. The list keeps the last one loaded |

`running` also covers the period after the scan closes while only preparation remains. That
period gets no separate state text or separate numbers (Requirements 1 and 2).

## Responsive Layout

The widths checked are 360 px, 768 px and 1280 px.

- 360 px:
  - Bottom-right indicator: fits on one line. When the issue counts do not fit, keep "M / N" and
    reduce the issue counts to icon and number (the words stay in the accessible name).
  - Summary: fits the same width as 012. The current activity line truncates on one line.
  - Settings list rows: the marker column becomes one column aligned to the start of the file
    name line. The text column uses the full screen width.
- 768 px and wider: list rows use two columns, marker and text. The marker column width fits the
  longer of "Failed" and "Check", so the text column starts at the same position on every row.
- 1280 px: the Settings section keeps its current maximum width (inside `max-w-4xl`). The list
  text column wraps at the section width.

At every width, these three keep their position and height:
- The bottom-right indicator
- The current activity line in the summary
- The current activity line in Settings

## Accessibility

Following the Issue's "assistive technology" item, 012's Accessibility changes as follows.

- `role="status"` announces only three milestones. Changes in the current activity or the
  progress are not announced.
  - Done: "The scan is complete." With to-check items, "N videos are worth checking." follows.
  - Some failed: "The scan finished with some failures. N videos may not be usable."
  - Failed: "The scan failed."
- The bar is named "Progress of the videos in this scan", with `aria-valuetext` "M of N videos
  done". During `finding`, `aria-valuenow` is not set (same as 012).
- The bottom-right indicator is named "〈state text〉 M of N videos done, 〈issue counts〉. Open the
  scan status".
- The issue list is a `ul`, and each row link is reachable with Tab.

## Review Criteria

At each of 360 px, 768 px and 1280 px, judge the following by eye.

1. **Hierarchy**: on every surface, the state text reads strongest, then the progress and issue
   counts, then the current activity line. Internal stage names (scan, preparation, job kinds;
   today's "Preparing", "Preparation left" and similar) do not appear as state text or number
   labels.
2. **One progress value**: throughout a 10-video import, the bottom-right and Settings numbers
   grow while staying "/ 10". They never switch to a number with a different unit. Finishing
   registration alone does not show "done" or a full bar.
3. **Never looks stalled**: during an import, the current activity line in the summary and in
   Settings advances with changing file names and action text. Meanwhile, the bottom-right
   indicator, the summary line and the Settings line keep their position and height.
4. **Usable or worth checking**: after the import, one glance at the bottom right shows "done" or
   "some failed" and the failed and to-check counts. In Settings, the list has as many rows as
   those counts.
5. **List readability**:
   - Failure rows come before to-check rows.
   - The marker word and icon distinguish failures from to-check items without color.
   - Long file names do not break the row, and the location line tells files with the same name
     apart.
   - A registered video's row goes to the player.
6. **Does not push content down**: with hundreds of issues, the Settings "Media folders" section
   stays in place. "Show more" reaches the last row.
7. **Density**: the Settings "Scan status" looks like the summary's elements plus only the
   denominator line, the failure reason and the list. No number fields absent from the summary
   appear.
8. **No overlap**: nothing overflows or overlaps in any of these combinations:
   - The bottom-right indicator and the summary
   - Toast
   - The player controls
   - Long file names
