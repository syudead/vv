# UI Design: single-account authentication and signed-out browsing of public videos

**Feature**: [parent Issue #135](https://github.com/syudead/vv/issues/135)

This design follows these sources and does not redecide them.

| Topic | Source |
| --- | --- |
| Visual rules, shell, list density | [Library UI: visual rules and list layout](../../docs/design-docs/library-ui.md) and `@theme` in [`web/src/index.css`](../../web/src/index.css) |
| Playback screen layout | [012 ui-design.md](../012-video-detail-ia/ui-design.md) |
| Toolbar, no-match and empty states | [013 ui-design.md](../013-library-search/ui-design.md) |
| Selection bar layout | [library-ui.md §6](../../docs/design-docs/library-ui.md#6-list-layout) |
| Tag display and operations | [014 ui-design.md](../014-video-tags/ui-design.md) |
| API, states and transitions | [contracts/auth-api.md](contracts/auth-api.md) (setup, sign-in, state, cookie) and [contracts/guest-api.md](contracts/guest-api.md) (guest response differences, visibility toggle) |
| Value rules | [data-model.md §6](data-model.md#6-username-and-password-values) |

This document defines only the setup screen, the sign-in screen, the gate, **what changes**
in existing screens between a guest (signed out) and the owner (signed in), the sign-in and
sign-out entries, and the visibility toggle. It adds no new color, radius or shadow tokens.
`pairs` in `tokens.test.ts` gains the pairs used on the setup and sign-in surfaces (see
"Accessibility").

## Screen boundary

- **Gate** (`web/src/auth/`, top of `App`): renders nothing until `GET /api/auth/session`
  answers. The answer selects one of three branches.
  - `setupRequired`: the setup screen (`/setup`) at any URL. The URL is replaced with
    `/setup`.
  - `guest`: the same shell and screen layout, degraded for guests. Owner-only screens
    (`/settings`, `/tags`) are replaced with `/login?next=<current URL>`. `/setup` is replaced
    with `/`, so a configured server never shows the setup screen again (Requirement 2).
  - `owner`: as today. Opening `/login` or `/setup` replaces the URL with `redirectTo` from
    `GET /api/auth/session?next=…` (`/` for `/setup`).
- **Setup screen (`/setup`) and sign-in screen (`/login`)**: one centered screen outside the
  shell (top bar and sidebar). See "Credential screens".
- **Shell**: the lower sidebar section shows "Settings" and "Sign out" for the owner, and
  "Sign in" for a guest. Guests do not see the right end of the top bar (refresh).
- **Library and folder screens (guest)**: controls that depend on owner data are **not
  shown**. They are not listed as disabled (UI quality "information density"). "Guest
  degradation" lists what is hidden.
- **Playback screen (`/videos/:id`)**: for the owner, the visibility toggle goes directly
  below the title-and-tags group under the title. For a guest, playback progress, tags,
  "Open file", "Copy path" and re-reading are hidden.
- **Selection bar (owner only)**: a "Visibility" menu follows the two tag operations.
- **Card (owner only)**: a public video gets a public mark inside the duration badge at the
  bottom right of the thumbnail.

## Gate

- Until the answer arrives, the gate renders nothing on `bg-bg`: no skeleton and no
  "Loading" text. The answer comes from one primary-key lookup, so it is never slow enough
  to need a waiting indicator.
- When the check fails, the center (`max-w-lg`, centered, as in `EmptyState`) shows:
  lucide `AlertCircle` (`text-danger`), the heading "Can't connect to the server"
  (`text-lg font-semibold`), the reason (`text-sm text-fg-muted`) and a secondary `Button`
  "Retry". Neither the list nor the playback screen renders. There is no automatic retry
  (Edge case "do not retry failed requests forever").
- While rendering as the owner, a 401 or an `X-VV-Audience: guest` response reloads the page
  once. No toast such as "Signed out" appears before the reload; the degraded guest screen
  after the reload is the result. On an owner-only screen, the gate after the reload sends
  the user to `/login?next=…`.

## Credential screens

The setup screen (`/setup`) and the sign-in screen (`/login`) share one skeleton.

### Layout

- The screen is `min-h-dvh bg-bg`. The content is centered horizontally and aligned to the
  top, with `pt-16` above (`pt-24` from `sm`) and `px-4` on the sides. It is not centered
  vertically, so a smartphone soft keyboard does not push the heading and inputs off screen.
- The content is one column, `w-full max-w-sm` (384 px), on a `bg-surface rounded-lg
  shadow-card` surface with `p-6` (`p-8` from `sm`). At 360 px, `px-4` of margin remains on
  both sides of the surface, so it does not touch the screen edges. At 1280 px, the column
  stays 384 px, so it does not spread out on wide screens.
- The column order, top to bottom:
  1. Identity
  2. Heading and supplement
  3. Inputs
  4. Failure line
  5. Primary action
  6. Connection warning
  7. (Setup only) guidance when the account is already configured
- Elements are `gap-5` apart; inputs are `gap-4` apart. The order follows UI quality "visual
  hierarchy": "identity → input → primary action → connection safety or failure reason".
- The identity is one line in the top bar's logo shape: a `size-2.5 rounded-full bg-accent`
  dot and "vv", `text-base font-semibold tracking-tight`. No further decoration, image or
  product description (UI quality "decoration and long explanations never stand out more
  than the primary action").

### Typography

| Element | Format |
| --- | --- |
| Heading (`h1`) | Setup "Create an account", sign-in "Sign in". `text-xl font-semibold text-fg` |
| Supplement | Setup only: "Create the one account for this server. To change it later, use the server's command line." `text-sm leading-6 text-fg-muted` |
| Input label | `text-xs font-medium text-fg-muted`, above the input (`mb-1`). Bound with `htmlFor` on `label` |
| Input | Same border and surface as the existing text input (`h-8 … px-2`), one step larger on this screen only, to match the primary action (`h-9 rounded-sm border border-border bg-field px-3 text-sm text-fg focus:border-accent focus:outline-none`). `w-full` |
| Failure line | `text-sm text-danger`, lucide `AlertCircle` (`size-4`) first, `role="alert"` |
| Primary action | `Button` primary, `lg`, `w-full`. Setup "Create account", sign-in "Sign in" |
| Connection warning | `text-sm leading-6 text-warning`, lucide `ShieldAlert` (`size-4`, `mt-1`) first, `border-l-2 border-warning-strong pl-3` on the left (the same shape as the "Change this folder?" dialog in Settings) |

Warnings and failures differ by icon and text as well as color (UI quality "typography").

### Fields

| Screen | Input | `name` / `autocomplete` | Type |
| --- | --- | --- | --- |
| Both | Username | `username` / `username` | `text`, `autocapitalize="none"`, `autocorrect="off"`, `spellcheck={false}` |
| Sign-in | Password | `password` / `current-password` | `password` |
| Setup | Password | `new-password` / `new-password` | `password` |
| Setup | Confirm password | `confirm-password` / `new-password` | `password` |

- The inputs sit inside a `form` without `method="post"`. `web/src/api/auth.ts` sends the
  request, and the default submit is stopped with `preventDefault`. The `form` lets Enter
  submit, and lets password managers recognize the username and password as one credential
  (Acceptance criterion 18).
- Initial focus is on the username. Tab order is DOM order: username → password →
  (confirmation) → primary action.
- There is no eye button to show the password. It would add one Tab stop, and with no
  strength rule the cost of a typo is small.
- `required` is not set. Browser validation bubbles do not match the design, and this
  screen's failure line carries all empty-field messages.

### Login behaviour

- While sending, the primary action is `disabled` with `LoaderCircle` (`animate-spin`) before
  its label. The inputs stay editable, so autofill is not disturbed. Double submits are
  blocked by `disabled` and by a sending flag that ignores Enter (Edge case "double
  submit").
- On success, the **whole page** navigates to `redirectTo` from the response
  (`window.location.assign`). An SPA transition is not used, so the copies read as a guest
  (such as `listSnapshot`) are discarded.
- 401 `invalid_credentials` (every failure, including empty fields) shows "The username or
  password is incorrect." on the failure line. Only the password input is cleared and
  focused; the username stays (UI quality "operation priority"). The UI never says which
  field is wrong (Requirement 12).
- 429 `login_throttled` shows "Too many sign-in attempts. Try again in N seconds." (N is
  `Retry-After`). There is no countdown; the text is fixed, and the primary action is
  `disabled` for N seconds. The password is not cleared.
- 403 (not same-origin), 400 and 5xx show "Couldn't sign in. Try again." A network failure
  shows "Can't connect to the server". All use the same failure line.
- An empty submit is still sent; the screen does not flag it first (Edge case "submit while
  empty"). Flagging is allowed, but the message could not name the field, so it would read
  the same as the failure.

### Setup behaviour

- The request is not sent when the confirmation does not match, the username or password is
  empty, or the username breaks [data-model.md §6](data-model.md#6-username-and-password-values).
  The failure line shows the reason and focus moves to that field.
- The messages are "Enter a username.", "Enter a password.", "The passwords don't match."
  and "Use a username of up to 128 characters, without leading or trailing spaces or control
  characters." Setup has no value to hide yet, so naming the field is allowed
  ([auth-api.md §2](contracts/auth-api.md#2-post-apiauthsetup)).
- On a mismatch, only the confirmation input is cleared.
- On success, the whole page navigates to `redirectTo` (`/`), which shows the owner's list
  (Acceptance criterion 1).
- 409 `account_already_configured` handling (Edge case "two devices at once"), so the losing
  side of a concurrent setup knows what to do next:
  1. The failure line shows "The account is already set up."
  2. All three inputs are cleared and the primary action becomes `disabled`.
  3. A `text-sm` "Go to sign in" (`text-link underline underline-offset-4`) appears at the
     end of the column, and focus moves to it.
- The link is a plain `a href="/login"`, not `Link`, and navigates the **whole page**. The
  gate on this screen still holds `setupRequired`, so an SPA transition would replace
  `/login` with `/setup` again. A full reload makes the gate call `GET /api/auth/session`
  again, receive `guest` and show the sign-in screen.
- 400 `invalid_request` shows `message` as is on the failure line.

### Connection warning

- When the page's `location.protocol` is not `https:`, both the setup and sign-in screens
  **always** show a warning directly below the primary action (Requirement 14). The text is
  "This connection isn't encrypted. Your username, password and sign-in status can be read in
  transit. Signing in doesn't protect against eavesdropping." It has no dismiss control.
- Over HTTPS, the line itself is absent. No positive text such as "This connection is
  secure" appears (UI quality "no wording that makes an HTTP connection look safe").
- `aria-describedby` on the username input and the primary action points to the warning, so
  it is read when the keyboard reaches the first input and just before submitting. It is not
  set on the `form`, because a `form` description is not read when focus moves to an input
  inside it. Although the warning is below the primary action in the DOM, assistive
  technology announces it first.

## Shell entries

### Sidebar

- The lower section (the `nav` with `aria-label="設定"`) becomes "Account and settings" and
  holds the following.

  | Audience | Lower section, top to bottom |
  | --- | --- |
  | Owner | "Settings" (unchanged) → "Sign out" (lucide `LogOut`) |
  | Guest | "Sign in" (lucide `LogIn`) |

- "Sign out" is a `button`, not a `NavLink`. It sends `POST /api/auth/logout` and, on `204`,
  navigates the whole page to **the current URL**. The reloaded screen is the degraded guest
  screen (Requirement 8); on an owner-only screen, the gate sends the user to
  `/login?next=…`.
- Sign-out has no confirmation dialog: signing in again undoes it, and no input is lost.
  While sending, the entry is `disabled` and reads "Signing out…". On failure, the toast
  "Couldn't sign out" appears and the entry returns to normal.
- "Sign in" is a `NavLink` to `/login?next=<current URL>`. The server validates `next` and
  turns it into `redirectTo`.
- The playback screen (`/videos/:id`) is theater mode outside the shell (012 ui-design.md)
  and has no sign-in or sign-out entry. Both happen from the shell sidebar. A guest who
  arrives from a shared URL returns to the list with × or Esc (`/` without `state.from`) and
  uses "Sign in" in the sidebar.
- The entries look like the existing ones (`Entry`). The expanded, rail and drawer states
  treat them the same. In the rail, the icon and a `text-[10px]` name stack vertically, as
  for the other entries.
- Guests do not see "Tags" in the upper section ("Guest degradation"). The upper section has
  two entries: "Library" and "Folders".

### Top bar

- Guests do not see the refresh (`ScanButton`) at the right end, or scan progress
  (`ScanProgressIndicator` and `ScanNoticeProvider` notices).
- For guests, `ScanProvider` does not call `GET /api/scans/current`, and nothing opens
  `/api/events`. None of the three users of `subscribeServerEvents` (`ScanProvider`,
  `useVideos`, `useVideoDetail`) subscribes for guests. Reason: `streamEvents` is "owner
  only", `EventSource` reconnects automatically even on 401, and every notice it carries
  (scans, playback progress, tags) belongs to the owner.
- The left edge and height of the center tools (`#topbar-library-tools`) do not change.
  Without the right-end container, the tools container (`flex-1`) grows to the right, and
  the tools center inside it.
- Compared with the owner screen, the tools shift right by half the refresh button's width;
  ☰ and the logo at the left do not move. No empty filler is added. UI quality "spacing
  rhythm" ("unnatural gaps") does not apply, because the space shrinks.

## Guest degradation

When rendering for a guest, the following are **not shown**. They are neither `disabled` nor
`aria-disabled` (UI quality "information density"; like library-ui.md §7, avoid "unavailable
now" marks).

| Place | Hidden | Instead |
| --- | --- | --- |
| Toolbar (library, folders) | The "Watch status" `fieldset` in the filters | The filter popover holds only "Playable only" and "Clear filters". The button count covers playability only |
| Toolbar | The "Recently played" sort | Six sorts remain. When `sort=playedDesc`, `sort=playedAsc`, `watch` or `tag` remains in the URL, the UI resets it to the default before requesting and fixes the URL ([guest-api.md §3](contracts/guest-api.md#3-filters-guests-cannot-use)). When the URL has no `sort` and the sort saved on the device (`sort` from `readViewPreferences`) is `playedAsc` or `playedDesc`, the UI checks the sort returned by `parseListCriteria` and resets it to the default `addedDesc` before requesting. The saved value is not rewritten, so it returns when the owner signs in again on the same device |
| Library card | The selection check (on hover and with `hover:none`), the tag row, the progress bar | The tag row is already hidden by the current rule, because `tags` is empty. The card ends at the title row with no leftover space (as for a video without tags today) |
| Library body | The active-tag row, the selection bar | — |
| Library list-view row | The selection check, watched state and progress | — |
| Empty state | "Scan", "Open Settings" | Heading "No videos are public", supplement "Sign in to see all videos.", secondary `Button` "Sign in" (`/login?next=` the current URL). The top of the folder screen uses the same text |
| Top of the folder screen | The path row on registered-folder cards (`showPath`) and the path in the accessible name | The display name comes from `FolderSummary.name`. The breadcrumb `title` also comes from `name`; no absolute path is built ([guest-api.md §1](contracts/guest-api.md#1-what-guests-see)) |
| Folder-screen search result card | The absolute path in the location `title` | `title` uses the same relative location as the display |
| Playback screen | The tag list (including input); "Open file" and "Copy path" in the facts row; "Read again", "Open file" and the `probeError` box of a read failure | Below the title, only the two rows of file facts and technical details remain ([012 ui-design "Video facts"](../012-video-detail-ia/ui-design.md#video-facts)). The response has no `location`, so the facts-row controls do not appear. The header breadcrumb is built from `folder`, so it appears. Playback starts from the beginning (no `progress`) |
| Playback screen | Saving playback progress (`saveProgress`, `beaconProgress`) | Not sent. The end-of-playback layer is unchanged |
| Sidebar | "Tags", "Settings" | "Sign in" ("Sidebar" above) |
| Top bar | Refresh, scan progress and notices | — |

- A video that stopped being public (`404`) on a guest playback screen uses the existing
  "video is gone" layer (`MissingVideo`) as is. The UI never says "It is now private"
  (Requirement 11).
- When the owner's session expires during playback (Edge case "session expires during
  playback"), `video` element load failures carry no 401 or `X-VV-Audience`. The playback
  screen handles it this way:
  1. When loading the video or the live transcode fails, the screen checks
     `GET /api/auth/session`.
  2. When the audience changed, the screen reloads the page once without showing a failure
     layer. The reloaded screen renders as a guest: a public video plays from the
     beginning, and a non-public one shows `MissingVideo`.
  3. When the audience did not change, the existing playback failure layer
     (`PlaybackFailure`) stays.
- The guest list never receives an owner-only response (401). If one arrives, the gate
  handles it as in "Gate".

## Visibility toggle

### Video page

- The visibility toggle is one row, left-aligned, `h-8`, directly below the title-and-tags
  group and above the file facts. It is `gap-2` from the group (the same as between title and
  tags) and `gap-5` from the file facts (the steps of 012's left column). The order is title
  → tags → visibility, and visibility looks weaker than tags.
- The control is a `button` with `role="switch"` and `aria-checked`. It holds an icon and a
  state label.

  | State | Icon | Label | Surface |
  | --- | --- | --- | --- |
  | Private | lucide `Lock` (`size-4`) | "Private" | `bg-elevated text-fg` (`Button` secondary, `sm`) |
  | Public | lucide `Globe` (`size-4`) | "Public" | `bg-accent-soft text-link` (the same as an active tag filter) |

  The accessible name is "Show to people who aren't signed in", and `aria-checked` conveys
  the state. The icon and the label change as well as the color (UI quality "distinguishable
  without relying on color").
- Pressing it sends `PUT /api/video-visibility` (`videoIds: [id]`). While sending, it is
  `disabled` and the icon becomes `LoaderCircle` (`animate-spin`). The state changes after
  the response arrives. No toast: the control's own label changes, and that is the result.
- On failure, "Couldn't change the visibility" in `text-sm text-danger` (`role="alert"`,
  `AlertCircle`) appears to the right of the row (below it under `sm`). It disappears on
  the next press or on moving to another video.
- The screen does not track changes made in another tab; refetching the video reflects them.

### Selection bar

- "Visibility" (lucide `Globe` + label + `ChevronDown`, `Button` ghost, `sm`) goes right
  after "Add tag" and "Remove tag", before the vertical divider.
- Pressing it opens `ui/Menu` upward. Today `MenuContent` accepts only `align`, so it gains
  `side`, as `PopoverContent` did in 014. The two items are "Make public" (`Globe`) and "Make
  private" (`Lock`).
- A menu is used instead of two buttons: from `sm`, the single row would hold four labeled
  buttons with the two tag operations. That group would be too long and would draw the eye
  before the divider to "Select all".
- The menu does not show the current state of the selected videos. `Video.public` shows it
  for selections on the loaded list. With "Select all", unread pages are included and their
  state is unknown, which would mean two kinds of display.
- Both items are always enabled; a video already in the requested state is not an error
  ([guest-api.md §4](contracts/guest-api.md#4-toggling-the-visibility-flag)).
- On confirm, the UI sends `PUT /api/video-visibility` and shows the toast "Made N videos
  public" or "Made N videos private" (N is `applied`). The selection stays. The card marks
  ("Card" below) change after the response arrives.
- On failure, the toast "Couldn't change the visibility" appears and the selection stays. The
  menu is already closed, so no in-popover row (the tag style) is used.
- When the selection exceeds the limit (20,000, the same as tags), "Visibility" is `disabled`
  with the same reason, as for the tag operations (library-ui.md §6).

### Card

- On an owner card, a public video gets lucide `Globe` (`size-3`, `text-fg`) at the start of
  the badge at the bottom right of the thumbnail (the "720P 59:11" surface), with `sr-only`
  "Public". A video with no badge (no quality and no duration) shows only `Globe` on the same
  surface.
- In the list view, the same icon goes right before the duration column.
- Guests do not see this mark. Every video a guest sees is public, so the mark means nothing.
- Visibility is not added to the card's primary information (thumbnail, title, duration,
  progress). The mark is one icon inside the duration surface and never draws the eye before
  the title or thumbnail.

## Interaction states

- Setup and sign-in primary action: normal (`bg-accent`) → hover (`bg-accent-hover`) →
  `disabled` (while sending, during a 429, after a 409; `opacity-50`). Focus uses the default
  outer outline.
- Inputs: `focus:border-accent`. The border color does not change after a failure, and
  `aria-invalid` is not set: the UI never says which field is wrong, so reddening both
  borders means nothing. Only setup validation sets `aria-invalid="true"` on the named field,
  with `aria-describedby` pointing to the failure line.
- Visibility toggle (`role="switch"`): normal → hover (secondary `hover:bg-hover-wash`;
  when public, it keeps `hover:bg-accent-soft` and the text becomes `text-fg`) → `disabled`
  (while sending).
- Sign-out entry: the same states as the existing `Entry`. `disabled` only while sending.

## Accessibility

- Each input is bound to a `label`, with `autocomplete` as in the table above (UI quality).
- The failure line and the already-configured guidance use `role="alert"` and are read when
  they appear. The 429 message too. The connection warning has no `role`; `aria-describedby`
  on the username input and the primary action binds it ("Connection warning").
- After a whole-page navigation, the destination's `h1` (on the list, the `sr-only`
  "Library") receives focus by the current rule. Sign-in success is not announced
  separately.
- `pairs` in `tokens.test.ts` gains the following. The setup and sign-in surfaces are
  `bg-surface`, and today only `fg` and `fg-muted` are checked on it.
  - `["danger", "surface", 4.5]` (failure line)
  - `["warning", "surface", 4.5]` (connection warning)
  - `["link", "surface", 4.5]` ("Go to sign in")
- Focus outlines are never removed. At 360 px, there is no horizontal scroll and no element
  overlap.
- The visibility toggle conveys its state with `aria-checked` on `role="switch"`, and the
  selection-bar menu items differ by label.
