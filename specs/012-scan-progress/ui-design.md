# UI Design: Scan progress display

**Feature**: [parent Issue #170](https://github.com/syudead/vv/issues/170)

> **This is not the source of truth for the current UI.** **What** the bottom-right
> display, the summary and the settings `取り込み状況` show was replaced by
> [the UI design for import progress and results](../024-import-progress/ui-design.md)
> (024). Of this document, only the placement and the interaction remain valid:
> one fixed display at the bottom right, pressing it goes to
> `/settings#scan-status`, hover and focus open the summary, the close button, the
> position of the settings section, and the handling of temporary fetch failures.
> For the wording rows in "Floating Indicator", the content of "Summary Popover",
> the count and time rows in "Settings Scan Status", and the table in "States",
> read 024.

Sources: the visual rules, the shell and the list density follow
[Library UI: visual rules and list layout](../../docs/design-docs/library-ui.md)
and the role tokens in [`web/src/index.css`](../../web/src/index.css). This
document defines only what the floating scan progress display and the settings
screen's `取り込み状況` add to them. No new colour, radius or shadow token is
added.

## Screen Boundary

- The floating progress display is placed once, above the routes, and the
  library, folder, settings and player screens share the same instance. On normal
  screens it is fixed at the bottom right. The player screen stays an independent
  theatre mode, and the progress display is fixed at the same bottom right as on
  normal screens. The player screen's top right holds the close × ([the video
  detail screen UI](../012-video-detail-ia/ui-design.md), "Close"), so the display
  does not go there.
- The top bar's `ライブラリを更新` stays as the start action when idle. The
  percentage, counts and failures during a scan are not shown in the top bar; the
  start button only briefly indicates that a scan is running.
- The settings screen places a section with `id="scan-status"` before the media
  folder settings. Opening `/settings#scan-status` directly also shows the current
  or most recent scan status first.
- The floating display and the settings section read the same state name,
  progress, counts, times and failure reason from the shared presentation model in
  `scanPresentation.ts`. No screen interprets `done + failed > 0` or an
  undetermined total on its own.

## Floating Indicator

The indicator is secondary operational state and looks weaker than the library
cards, search, filters, folder navigation and the way into playback. On every
screen and at every width it sits `fixed` at the bottom right and does not take
part in the body's layout or scrolling. `app/App.tsx` owns the route-type check,
and it switches only how the Toast is placed.

| Situation | Indicator |
| --- | --- |
| Idle, or no scan has ever run | Not shown |
| Running | Icon, state name and, only when determined, the percentage, in one line. While the total is 0 or undetermined, no percentage; the text reads as `確認中` or `件数確認中` |
| Completed or partly failed | Switches to the result in the same place, shows it for 8 seconds, then closes automatically. While the summary is read through hover or focus it does not close; the remaining time resumes after the pointer or focus leaves. A partial failure shows the failure through the count and the text |
| Failed entirely | Stays until the user closes it or goes to the settings details |

- Only on failure, a small icon button appears inside the indicator, with the
  accessible name `取り込み失敗の通知を閉じる`. In the Tab order it comes right after
  the trigger that goes to the settings details, and Enter or Space closes it. The
  failure state combines text and an icon and does not rely on colour alone.
- On the `bg-elevated` surface, failure is emphasised with `text-fg` and an icon,
  `border-border-strong` and, when needed, a `danger-soft` wash. `text-danger` is
  limited to supporting text or badges on `bg`. When `danger` is used as text on
  `elevated` or `surface`, add it to the contrast pairs in `tokens.test.ts`.
- The surface uses `bg-elevated`, `border-border-strong` and `shadow-elevated`.
  The corner radius stays at most `rounded-md`, matching the existing compact
  controls; it is not a large card like a page section.
- At 360px it keeps a safe margin from the right and bottom edges. When it
  appears together with an existing Toast, it moves above the Toast so that both
  texts are readable. On the player screen it does not overlap the close ×.

## Summary Popover

Hover and keyboard focus open the same summary. On devices without hover,
activation leads to the settings details, so the summary is not required to
operate anything.

- The content is only the state, a progress bar, the processed count, the total
  count and the failure count. The start time, end time, failure reason and retry
  belong to the settings section.
- The progress bar is determinate while running with a total determined as 1 or
  more, on completion and on partial failure. While the total is undetermined,
  while the start request is pending and during a temporary fetch failure, it is
  indeterminate, and no number suggesting 0% or 100% is shown.
  `done(total=0, completed=0)` shows the text and time of the completed state, but
  the summary shows no determinate bar or 100%.
- The popover normally opens towards the centre of the screen; from the bottom
  right it expands to the upper left. Radix collision handling keeps it on screen.
  On the 360px player screen it opens into the free area below the player and
  keeps a width that does not cover the body for long. Counts do not wrap and read
  like a table.
- Moving the pointer from the trigger to the popover does not close it. Escape,
  moving focus and pointer leave close it.
- Click, tap, Enter and Space on the trigger navigate to `/settings#scan-status`;
  they do not toggle the popover.

## Settings Scan Status

`取り込み状況` is operational information on the settings screen, placed weaker
than the page title and readable before the media folder settings. It inherits
the existing sections' padding, dividers and text sizes, and is not enlarged like
a separate dashboard.

Top to bottom:

1. The heading `取り込み状況` and a short status badge for the current state.
2. The state description and the progress bar. When nothing has run and for
   `done(total=0, completed=0)`, no empty or 100% bar is shown; the two are told
   apart by the texts `まだ取り込んでいません` and `対象はありませんでした`.
3. The count row: processed, total and failed, aligned with tabular numbers. An
   undetermined total reads `確認中`, distinct from 0.
4. The time row: the start time and the end time. An undetermined value shows
   text such as `未完了` instead of a blank.
5. Only on total failure: the reason, which may wrap, and the primary action
   `再試行`. A long reason wraps within the section width and does not push the
   action out.

During a temporary fetch failure, when an earlier scan was fetched, it stays and
`最新状態を再確認中` is added as a note. When there is no earlier scan, the section
shows the fetch-failure text and that it is retrying automatically, instead of an
empty progress bar.

## States

| State | Floating indicator | Summary popover | Settings section |
| --- | --- | --- | --- |
| Never run | Not shown | None | `まだ取り込んでいません`. No progress bar |
| Start requested | `開始中` | Indeterminate, counts being checked | Auto-refreshing as start requested |
| Running, total undetermined | `取り込み中` | Indeterminate, processed and failed counts | Running, total being checked |
| Running, total determined | `取り込み中 NN%` | Determinate, state and counts | The same percentage and counts, start time |
| Completed with 0 items | `完了` for 8 seconds | No bar, nothing to scan | Nothing to scan, start time, end time |
| Completed | `完了` for 8 seconds | Determinate, final counts | Completed, start time, end time |
| Partly failed | `一部失敗` while the timer runs | Determinate, failure count | Partly failed, final counts, times |
| Failed entirely | `失敗` until acknowledged, close button | Indeterminate or final counts, failure count | Failure reason and retry |
| Temporary fetch failure | With a known scan, keeps its last state and adds a weak note. Without a known scan, not shown | Re-checking the latest state | The last state, or retrying automatically |

## Responsive Layout

| Width | Layout |
| --- | --- |
| 360px | Narrow enough to fit at the bottom right on normal screens and the player screen, without overlapping the body, player controls, video information, detail tabs, mobile navigation or Toasts. On the player screen the popover opens into the free area below the player, and long text wraps naturally to at most two lines. The settings section is one column |
| 768px | At the bottom right, where it does not visually compete with the sidebar or the toolbar. On the player screen also at the bottom right, avoiding the close × at the player's top right. The popover is wide enough to read the counts as roughly two columns and does not keep covering the main controls |
| 1280px | The indicator stays small, as a helper at the edge of the screen. The popover keeps a density that does not look like a large card and does not upset the primacy of the list grid |

At 200% zoom and with long failure reasons, the indicator label may wrap, and the
settings section's reason stays inside the section with the equivalent of
`break-words`.

## Accessibility

- The indicator trigger is a button or a button-like element, and its accessible
  name includes the state and the destination, for example
  `取り込み中 70%。取り込み状況を開く`.
- The trigger is reachable with Tab, and its focus-visible outline uses the
  existing `link` colour. Enter and Space navigate to `/settings#scan-status`.
- The summary does not rely on hover alone; focus shows the same content. Escape
  closes it.
- Progress announcements use `role="status"` and do not read every state update.
  Meaningful milestones come first: state changes, the total becoming determined,
  completion, partial failure and total failure.
- The progress bar has an accessible name and value. When indeterminate it has no
  `aria-valuenow`, and the text conveys that the total is undetermined.
- States are told apart by text and lucide icons in addition to colour. Added
  icons are line icons close to the existing set of `RefreshCw`, `CheckCircle2`,
  `AlertTriangle` and `XCircle`.
- On a hash navigation to the settings section, the section with
  `id="scan-status"` is the scroll target, and programmatic focus moves to the
  heading inside it. The heading has `tabindex="-1"` and may show a visible focus
  ring. After click, Enter or Space, the `取り込み状況` heading or the status
  summary right after it is the active element. The browser's standard hash
  scroll is respected, and no extra page-level scroll owner is added.
