# UI Design: scan progress display

**Feature**: [parent Issue #170](https://github.com/syudead/vv/issues/170)

> **Not the source of truth for the current UI.** What the bottom-right indicator, the summary and
> the Settings "Scan status" **show** was replaced by
> [the scan progress and result UI design](../024-import-progress/ui-design.md) (024). Only the
> placement and interaction in this document still hold: one indicator fixed at the bottom right,
> activation moves to `/settings#scan-status`, hover and focus open the summary, the close button,
> the position of the Settings section, and the handling of transient fetch failures. For the text
> rows of "Floating Indicator", the content of "Summary Popover", the count and time rows of
> "Settings Scan Status", and the "States" table, read 024.

Visual rules, the shell and list density follow
[Library UI: visual rules and list layout](../../docs/design-docs/library-ui.md) and the role tokens
in [`web/src/index.css`](../../web/src/index.css). This document defines only what the floating
scan progress display and the Settings "Scan status" add to them. No new color, radius or shadow
tokens are added.

## Screen Boundary

- The floating progress display is placed once, above the routes. The library, folder, Settings
  and playback pages share the same display instance. On normal pages it is fixed at the bottom
  right.
- The playback page stays an independent theater mode, and the progress display is fixed at the
  same bottom right as on normal pages. The top right of the playback page holds the close ×
  ([video detail page UI](../012-video-detail-ia/ui-design.md) "Close"), so the display does not go
  there.
- "Refresh library" in the top bar stays as the start action while idle. The percentage, counts
  and failure results of a running scan do not appear in the top bar; the start button only
  briefly indicates that a scan is running.
- Settings places the `id="scan-status"` section before the media folder settings. Opening
  `/settings#scan-status` directly also shows the current or latest scan status first.
- The floating display and the Settings section read the same state names, progress, counts, times
  and failure reason from the shared display model in `scanPresentation.ts`. No page interprets
  `done + failed > 0` or an unknown total on its own.

## Floating Indicator

The indicator is a secondary operational status. It looks weaker than the paths to library cards,
search, filters, folder navigation and playback. On every page and at every width it is `fixed` at
the bottom right and does not take part in the body layout or scroll. `app/App.tsx` owns the route
kind check and switches only how the Toast is placed.

- Hidden while idle and when no scan has ever run.
- While running, one line shows an icon, the state name, and the percentage only when it is known.
  While the total is 0 or unknown, no percentage is shown; the text reads "Checking" or
  "Counting".
- Done and partial failure switch to a result display in the same position, shown for 8 s and then
  closed automatically. It stays open while the summary is read through hover or focus, and the
  remaining time resumes after the pointer or focus leaves. A partial failure shows the failure
  through a count and text.
- A total failure stays until the user dismisses it or goes to the Settings details. Only on
  failure, a small icon button appears inside the indicator, with the accessible name "Dismiss the
  scan result notice".
- The dismiss button comes right after the trigger to the Settings details in Tab order, and Enter
  or Space dismisses it. The failure state combines text and an icon and does not rely on color
  alone.
- On a `bg-elevated` surface, failure emphasis uses `text-fg` with an icon, `border-border-strong`,
  and a `danger-soft` wash if needed. `text-danger` is limited to secondary text or badges on `bg`.
  Using `danger` as text on `elevated` or `surface` requires adding the pair to the contrast pairs
  in `tokens.test.ts`.
- The surface uses `bg-elevated`, `border-border-strong` and `shadow-elevated`. Corners stay at
  `rounded-md` at most, matching existing compact controls, and it is not a large card like a page
  section.
- At 360 px it keeps a safe margin from the right and bottom edges. When it appears together with
  an existing Toast, it moves above the Toast so both texts stay readable. On the playback page it
  does not overlap the close ×.

## Summary Popover

Hover and keyboard focus open the summary with the same content. On devices without hover,
activation leads to the Settings details, so the summary is not a precondition for any action.

- Content is limited to the state, a progress bar, the processed count, the total count and the
  failure count. Start time, finish time, failure reason and retry go to the Settings section.
- The progress bar is determinate while running with a known total of 1 or more, when done, and on
  partial failure. With an unknown total, a pending start request or a transient fetch failure, it
  is indeterminate and shows no number that suggests 0% or 100%.
- `done(total=0, completed=0)` shows the text and time of the done state, but the summary shows no
  determinate bar and no 100%.
- The popover opens toward the center of the screen by default; at the bottom right it expands
  toward the top left. Radix collision handling keeps it on screen. On the playback page at 360 px
  it opens into the free area below the player, and its width does not cover the body for long.
  Counts do not wrap and read like a table.
- Moving the pointer from the trigger to the popover does not close it. Escape, a focus move and
  pointer leave close it.
- Click, tap, Enter and Space on the trigger move to `/settings#scan-status` instead of toggling
  the popover.

## Settings Scan Status

"Scan status" is operational information on the Settings page. It is weaker than the page title and
is readable before the media folder settings. It inherits the spacing, dividers and font sizes of
the existing sections and does not grow into a standalone dashboard.

From top to bottom:

1. The heading "Scan status" and a short status badge for the current state.
2. The state description and the progress bar. Not-run and `done(total=0, completed=0)` show no
   empty or 100% bar; the texts "No scan has run yet" and "No changed files were found." tell them
   apart.
3. The count row: processed, total and failed, aligned with tabular numbers. An unknown total reads
   "Checking", distinct from 0.
4. The time row: start time and finish time. A value not yet known shows text such as "Not
   finished" instead of a blank.
5. Only on total failure: the reason, which can wrap, and the primary action "Retry". A long reason
   wraps within the section width and does not push the action out.

During a transient fetch failure, if a scan was fetched before, it stays and "Rechecking the latest
status" appears as a supplement. If there is no previous scan, the section shows fetch-failure text
and that automatic retry is in progress, instead of an empty progress bar.

## States

| State | Floating indicator | Summary popover | Settings section |
| --- | --- | --- | --- |
| Not run | Hidden | None | "No scan has run yet". No progress bar |
| Start requested | "Starting" | Indeterminate, counting | Auto-updating as start requested |
| Running, total unknown | "Scanning" | Indeterminate, processed and failed counts | Running, total count being checked |
| Running, total known | "Scanning NN%" | Determinate, state and counts | Same percentage and counts, start time |
| Done with 0 items | "Done" for 8 s | No bar, nothing to scan | Nothing to scan, start time, finish time |
| Done | "Done" for 8 s | Determinate, final counts | Done, start time, finish time |
| Partial failure | "Some failed" only while the timer runs | Determinate, failure count | Partial failure, final counts, times |
| Total failure | "Scan failed" until acknowledged, dismiss button | Indeterminate or final counts, failure count | Failure reason and retry |
| Transient fetch failure | With a known scan, keeps the last state and shows only a weak supplement. Without one, hidden | Rechecking the latest status | Last state, or automatic retry in progress |

## Responsive Layout

- 360 px: on normal and playback pages the indicator fits at the bottom right. It does not overlap
  the body, player controls, video information, detail tabs, mobile navigation or the Toast. On the
  playback page the popover opens into the free area below the player, and long text wraps
  naturally to at most 2 lines. The Settings section is one column.
- 768 px: placed at the bottom right, where it does not compete visually with the sidebar or
  toolbar. On the playback page it is also at the bottom right and avoids the close × at the top
  right of the player. The popover is wide enough to read counts in about 2 columns and does not
  keep covering the main actions.
- 1280 px: the indicator stays small, as a secondary display at the screen edge. The popover keeps
  a density that does not look like a large card, and does not upset the primacy of the list grid.
- At 200% zoom and with long failure reasons, the indicator label may wrap, and the reason in the
  Settings section stays within its box with the equivalent of `break-words`.

## Accessibility

- The indicator trigger is a button or a button-equivalent element. Its accessible name includes
  the state and the destination, for example "Scanning 70%. Open the scan status".
- The trigger is reachable with Tab, and its focus-visible outline uses the existing `link` color.
  Enter and Space move to `/settings#scan-status`.
- The summary does not depend on hover alone; focus shows the same content. Escape closes it.
- Progress announcements use `role="status"` and do not read every update. Meaningful milestones
  come first: a state change, the total becoming known, done, partial failure and total failure.
- The progress bar has an accessible name and value. When indeterminate it has no `aria-valuenow`,
  and text conveys that the total is unknown.
- States are distinguished by text and a lucide icon in addition to color. Added icons are line
  icons close to the existing set of `RefreshCw`, `CheckCircle2`, `AlertTriangle` and `XCircle`.
- On a hash navigation to the Settings section, the `id="scan-status"` section is the scroll
  target, and programmatic focus moves to its heading. The heading has `tabindex="-1"` and may show
  a visible focus ring.
- After click, Enter or Space, the active element is the "Scan status" heading or the status
  summary right after it. The browser's standard hash scroll is respected, and no extra page-level
  scroll owner is added.
