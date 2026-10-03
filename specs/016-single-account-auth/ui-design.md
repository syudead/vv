# UI design: Single-account authentication and browsing public videos without logging in

**Feature**: [parent Issue #135](https://github.com/syudead/vv/issues/135)

Sources: visual rules, the shell and list density follow
[Library UI: visual rules and list structure](../../docs/design-docs/library-ui.md)
and the `@theme` in [`web/src/index.css`](../../web/src/index.css). The video
page structure follows [012's ui-design.md](../012-video-detail-ia/ui-design.md);
the toolbar, the no-match state and empty states follow
[013's ui-design.md](../013-library-search/ui-design.md); the selection bar
structure follows
[library-ui.md §6](../../docs/design-docs/library-ui.md#6-一覧の構成); tag
display and actions follow [014's ui-design.md](../014-video-tags/ui-design.md).
The APIs, states and transitions the screens use are fixed in
[contracts/auth-api.md](contracts/auth-api.md) (first-time setup, login, state,
cookies) and [contracts/guest-api.md](contracts/guest-api.md) (guest response
differences, public toggle); this document does not revisit them. Value rules
are in [data-model.md §6](data-model.md#6-username-and-password-values).

This document defines only the first-time setup screen, the login screen, the
gate, **what changes** in existing screens between a guest (not logged in) and
the owner (logged in), the login and logout entries, and the public toggle. No
new color, radius or shadow tokens are added. `pairs` in `tokens.test.ts` gains
the pairs used on the first-time setup and login screen surfaces
("Accessibility" below).

## Screen boundary

- **Gate** (`web/src/auth/`, the top of `App`): draws nothing until
  `GET /api/auth/session` answers. The answer splits into three:

| Answer | Screen |
| --- | --- |
| `setupRequired` | The first-time setup screen (`/setup`) at every URL; the URL is replaced with `/setup` |
| `guest` | The same shell and screen structure, degraded for guests. Owner-only screens (`/settings`, `/tags`) are replaced with `/login?next=<current URL>`. `/setup` is replaced with `/` (a configured server never shows the first-time setup screen again, requirement 2) |
| `owner` | As now. Opening `/login` or `/setup` is replaced with `redirectTo` from `GET /api/auth/session?next=…` (`/` for `/setup`) |

- **First-time setup screen (`/setup`) and login screen (`/login`)**: one
  centered screen outside the shell (top bar, sidebar) ("Credential screens"
  below).
- **Shell**: the bottom of the sidebar shows `設定` and `ログアウト` for the
  owner and `ログイン` for guests. The top bar's right end (refresh) is hidden
  for guests.
- **Library and folder screens (guest)**: controls that depend on the owner's
  data are **not shown**, rather than listed as disabled (`UI品質`
  "information density"). "Guest degradation" lists what is hidden.
- **Video page (`/videos/:id`)**: for the owner, the public toggle is added
  directly below the title and tag group. Guests do not see playback position,
  tags, `ファイルを開く`, `パスをコピー`, or re-reading.
- **Selection bar (owner only)**: a `公開` menu is added right after the two tag
  actions.
- **Card (owner only)**: a public video gets a public mark inside the duration
  badge at the thumbnail's bottom right.

## Gate

- Until the answer arrives the screen stays `bg-bg` with nothing drawn: no
  skeleton and no loading text. The answer is one primary key lookup, never long
  enough to need a waiting indicator.
- When the check fails, the center (`max-w-lg` and centered, as `EmptyState`)
  shows lucide `AlertCircle` (`text-danger`), the heading
  `サーバーに接続できません` (`text-lg font-semibold`), the reason
  (`text-sm text-fg-muted`) and a secondary `Button` `再試行`. Neither the list
  nor the video page is drawn. There is no automatic retry (Edge Case
  `失敗した要求を無限に再試行しない`).
- While drawing as the owner, receiving 401 or `X-VV-Audience: guest` reloads
  the page once. No toast such as `ログアウトされました` appears before the
  reload: the degraded guest screen after the reload is itself the result. On
  an owner-only screen, the gate after the reload sends the user to
  `/login?next=…`.

## Credential screens

The first-time setup screen (`/setup`) and the login screen (`/login`) share one
skeleton.

### Layout

- The whole screen is `min-h-dvh bg-bg`. Content is centered horizontally and
  top-aligned, with top padding `pt-16` (`pt-24` from `sm`) and `px-4` on the
  sides. It is not vertically centered so the heading and inputs are not pushed
  off screen when a phone's soft keyboard appears.
- Content is one `w-full max-w-sm` (384px) column on a
  `bg-surface rounded-lg shadow-card` surface with `p-6` (`p-8` from `sm`). At
  360px, `px-4` remains on both sides of the surface so it does not touch the
  screen edges. At 1280px the column stays 384px and does not spread out.
- From the top the column holds: identity → heading and note → inputs →
  failure line → primary action → connection warning → (first-time setup only)
  the already-configured notice, with `gap-5` between elements and `gap-4`
  between inputs. This is the `UI品質` "visual hierarchy" order "identity →
  input → primary action → connection safety or failure reason".
- Identity is one line in the same form as the top bar logo (a
  `size-2.5 rounded-full bg-accent` dot and `vv`,
  `text-base font-semibold tracking-tight`). No further decoration, images or
  product description (`UI品質` "decoration and long explanations do not stand
  out more than the primary action").

### Typography

| Element | Style |
| --- | --- |
| Heading (`h1`) | Setup `アカウントを作成`, login `ログイン`. `text-xl font-semibold text-fg` |
| Note | Setup only: `このサーバーを使うアカウントを1つ作ります。あとから変えるにはサーバーのコマンドを使います`. `text-sm leading-6 text-fg-muted` |
| Input label | `text-xs font-medium text-fg-muted`, above the input (`mb-1`). Tied with `htmlFor` on `label` |
| Input | Same frame and surface as the existing text input (`h-8 … px-2`), one step larger on this screen only to match the primary action (`h-9 rounded-sm border border-border bg-field px-3 text-sm text-fg focus:border-accent focus:outline-none`). `w-full` |
| Failure line | `text-sm text-danger`, leading lucide `AlertCircle` (`size-4`), `role="alert"` |
| Primary action | Primary `Button`, `lg`, `w-full`. Setup `設定してはじめる`, login `ログイン` |
| Connection warning | `text-sm leading-6 text-warning`, leading lucide `ShieldAlert` (`size-4`, `mt-1`), `border-l-2 border-warning-strong pl-3` on the left (the same form as the settings page's `フォルダの変更を確認` dialog) |

Warnings and failures differ by icon and wording as well as color (`UI品質`
"typography").

### Fields

| Screen | Input | `name` / `autocomplete` | Type |
| --- | --- | --- | --- |
| Both | Username | `username` / `username` | `text`, `autocapitalize="none"`, `autocorrect="off"`, `spellcheck={false}` |
| Login | Password | `password` / `current-password` | `password` |
| Setup | Password | `new-password` / `new-password` | `password` |
| Setup | Password (confirm) | `confirm-password` / `new-password` | `password` |

- The inputs are inside a `form` without `method="post"` (`web/src/api/auth.ts`
  submits, and the default submit is `preventDefault`ed). A `form` lets Enter
  submit and lets password managers recognize the username and password as one
  credential pair (acceptance criterion 18).
- Initial focus is the username. Tab order is DOM order (username → password →
  (confirm) → primary action). There is no eye button to show the password: it
  would add a Tab stop, and with no strength rule the cost of a typo is small.
- No `required`. The browser's validation bubbles do not match the visual
  style, and empty fields are reported through this screen's failure line like
  everything else.

### Login behaviour

- While submitting, the primary action is `disabled` with a leading
  `LoaderCircle` (`animate-spin`). Inputs stay editable (so autofill is not
  disturbed). Double submits are prevented by `disabled` and by ignoring Enter
  while a submitting flag is set (Edge Case `二重送信`).
- On success the **whole page** navigates to the response's `redirectTo`
  (`window.location.assign`). It is not an SPA transition so that snapshots
  read as a guest (`listSnapshot` and others) are discarded.

| Response | Failure line and behaviour |
| --- | --- |
| 401 `invalid_credentials` (every failure, including empty fields) | `ユーザー名またはパスワードが違います`. Only the password input is cleared and focused; the username stays (`UI品質` "action priority"). It does not say which field is wrong (requirement 12) |
| 429 `login_throttled` | `試行が多すぎます。N 秒後にやり直してください` (N is `Retry-After`). No countdown; the wording is fixed and the primary action is `disabled` for N seconds. The password is not cleared |
| 403 (not same-origin), 400, 5xx | `ログインできませんでした。もう一度お試しください` |
| Network failure | `サーバーに接続できません` |

- Submitting with empty fields still sends the request; the screen does not
  flag it first (Edge Case `空のまま送信`. Flagging is allowed, but since it
  cannot say which field, the wording would be the same as the failure).

### Setup behaviour

- When the confirmation password does not match, the username or password is
  empty, or the username breaks
  [data-model.md §6](data-model.md#6-username-and-password-values), nothing is
  sent; the failure line gives the reason and focus moves to that field. The
  wording is `ユーザー名を入力してください`, `パスワードを入力してください`,
  `確認用のパスワードが一致しません`, or
  `ユーザー名は 128 文字まで、前後の空白と制御文字なしにしてください`. During
  first-time setup there is nothing to hide yet, so naming the field is allowed
  ([auth-api.md §2](contracts/auth-api.md#2-post-apiauthsetup)).
- On mismatch only the confirmation input is cleared.
- On success the whole page navigates to `redirectTo` (`/`), showing the
  owner's list (acceptance criterion 1).
- 409 `account_already_configured` shows `アカウントは既に設定されています` in
  the failure line, clears all three inputs, makes the primary action
  `disabled`, and shows `ログインへ` in `text-sm`
  (`text-link underline underline-offset-4`) at the end of the column. Focus
  moves to this link, so the side that lost a concurrent first-time setup knows
  what to do next (Edge Case `2つの端末が同時に`). The link is a plain
  `a href="/login"`, not `Link`, and navigates the **whole page**: this
  screen's gate still remembers `setupRequired`, so an SPA transition would
  replace `/login` with `/setup` again. A full reload makes the gate call
  `GET /api/auth/session` again, receive `guest`, and show the login screen.
- 400 `invalid_request` shows `message` as is in the failure line.

### Connection warning

- When the page's `location.protocol` is not `https:`, both the first-time
  setup and login screens **always** show a warning directly below the primary
  action (requirement 14). The wording is
  `この接続は暗号化されていません。ユーザー名、パスワード、ログイン状態は通信路で読み取られるおそれがあります。ログインは通信の盗聴を防ぎません`.
  It cannot be dismissed.
- Over HTTPS the line is absent. No reassuring text such as
  `安全な接続です` appears either (`UI品質` "wording that makes an HTTP
  connection look safe is not allowed").
- The username input and the primary action point at the warning with
  `aria-describedby`, so it is read when the keyboard first reaches the first
  input and just before submitting. It is not on the `form` itself (a `form`'s
  description is not read when focus moves to an input inside it). Although it
  sits below the primary action in the DOM, assistive technology hears it
  first.

## Shell entries

### Sidebar

- The bottom group (`nav` with `aria-label="設定"`) becomes
  `アカウントと設定` and holds:

  | Audience | Bottom group (top to bottom) |
  | --- | --- |
  | Owner | `設定` (unchanged) → `ログアウト` (lucide `LogOut`) |
  | Guest | `ログイン` (lucide `LogIn`) |

- `ログアウト` is a `button`, not a `NavLink`. It sends
  `POST /api/auth/logout` and on `204` navigates the whole page to the
  **current URL**. The reloaded screen is the degraded guest screen
  (requirement 8), and on an owner-only screen the gate sends the user to
  `/login?next=…`. There is no confirmation dialog: logging in again undoes it,
  and no input is lost. While sending, the item is `disabled` with the wording
  `ログアウト中…`. On failure a toast says `ログアウトできませんでした` and the
  item reverts.
- `ログイン` is a `NavLink` to `/login?next=<current URL>` (the server validates
  `next` into `redirectTo`).
- The video page (`/videos/:id`) is theater mode outside the shell (012's
  ui-design.md) and has no login or logout entry; both go through the shell
  sidebar. A guest arriving directly from a shared URL returns to the list
  with × or Esc (`/` when there is no `state.from`) and uses `ログイン` in the
  sidebar.
- The look matches existing items (`Entry`). The expanded, rail and drawer
  states treat it the same; in the rail it has the icon and a `text-[10px]`
  name stacked vertically like the other items.
- Guests do not see `タグ` in the top group ("Guest degradation"), so the top
  group has two items, `ライブラリ` and `フォルダ`.

### Top bar

- Guests do not see the refresh at the right end (`ScanButton`) or import
  progress (`ScanProgressIndicator` and `ScanNoticeProvider` notifications).
  For guests `ScanProvider` does not call `GET /api/scans/current`, and nothing
  opens `/api/events`. None of the three users of `subscribeServerEvents`
  (`ScanProvider`, `useVideos`, `useVideoDetail`) subscribes for guests:
  `streamEvents` is "Owner only", `EventSource` reconnects automatically even on
  401, and every notice it carries (import, playback position, tags) belongs to
  the owner.
- The left edge and height of the central tools (`#topbar-library-tools`) do
  not change. Without the right-end container, the tools container (`flex-1`)
  extends to the right and the tools stay centered in it. Compared with the
  owner's screen the tools shift right by half the refresh button's width, but
  ☰ and the logo at the left do not move. No filler is added (`UI品質`
  "spacing rhythm"'s "unnatural blank space" does not occur, since the space
  tightens rather than widens).

## Guest degradation

When drawing as a guest, the following are **not shown**. They are neither
`disabled` nor `aria-disabled` (`UI品質` "information density"; the
"unavailable right now" mark is avoided for the same reason as library-ui.md
§7).

| Place | Hidden | Instead |
| --- | --- | --- |
| Toolbar (library, folders) | The filter's watch state `fieldset` | The filter popover holds only `再生できるものだけ` and `条件を解除`. The button's number counts only playability |
| Toolbar | The sort option `最近再生した順` | Six sort options remain. When `sort=playedDesc`, `sort=playedAsc`, `watch` or `tag` remains in the URL, they are rounded to the defaults before requesting, and the URL is corrected ([guest-api.md §3](contracts/guest-api.md#3-conditions-guests-cannot-use)). When the URL has no `sort` and the sort saved on the device (`sort` of `readViewPreferences`) is `playedAsc` or `playedDesc`, the sort returned by `parseListCriteria` is checked and rounded to the default `addedDesc` before requesting. The saved value itself is not rewritten (it returns when the owner logs in again on the same device) |
| Library cards | The selection checkbox (on hover and with `hover:none`), the tag row, the playback progress bar | The tag row is absent under the current rule because `tags` is empty. The card ends at the title line with no blank row (as a video without tags today) |
| Library body | The active tag filter row, the selection bar | — |
| Library list view rows | The selection checkbox, watched and progress | — |
| Empty state | `取り込む`, `設定を開く` | Heading `公開されている動画はありません`, note `ログインすると、すべての動画を見られます`, secondary `Button` `ログイン` (`/login?next=` the current URL). The top level of the folder screen uses the same wording |
| Top level of the folder screen | The path line on registered folder cards (`showPath`), the path in accessible names | The display name comes from `FolderSummary.name`. The breadcrumb `title` is also built from `name`, never assembling an absolute path ([guest-api.md §1](contracts/guest-api.md#1-what-guests-see)) |
| Folder screen search result cards | The absolute path in the location's `title` | `title` is the same relative location as the display |
| Video page | The tag list (including the input), `ファイルを開く` and `パスをコピー` in the facts line, the read failure's `もう一度読み取る` and `ファイルを開く`, and the `probeError` box | Below the title there are only the two lines of file facts and technical details ([012's ui-design "Video facts"](../012-video-detail-ia/ui-design.md#video-facts)). The response has no `location`, so the facts line has no actions. The breadcrumb in the header band is built from `folder`, so it appears. Playback starts at the beginning (no `progress`) |
| Video page | Saving playback position (`saveProgress`, `beaconProgress`) | Not sent. The end-of-playback layer stays as now |
| Sidebar | `タグ`, `設定` | `ログイン` ("Sidebar" above) |
| Top bar | Refresh, import progress and notifications | — |

- On a guest's video page, a video that is no longer public (`404`) uses the
  existing "video disappeared" layer (`MissingVideo`) as is. It does not say
  `非公開になりました` (requirement 11).
- When the owner's session expires during playback (Edge Case
  `再生中にセッションが失効`), a failed `video` element load carries neither
  401 nor `X-VV-Audience`. The video page therefore checks
  `GET /api/auth/session` when loading the video or live transcode fails; when
  the audience has changed, it reloads the page once without showing the
  failure layer. The reloaded screen draws as a guest: a public video plays from
  the beginning, and a non-public one becomes `MissingVideo`. When the audience
  has not changed, the current playback failure layer (`PlaybackFailure`)
  stays.
- An owner-only response (401) does not happen in a guest's list; if it did,
  the gate handles it as in "Gate".

## Visibility toggle

### Video page

- One left-aligned `h-8` line directly below the title and tag group, above the
  file facts. The gap to the group is `gap-2` (as between title and tags), and
  to the file facts `gap-5` (012's left column step). The order is title →
  tags → public, with public looking weaker than tags.
- The component is a `button` with `role="switch"` and `aria-checked`. It holds
  an icon and a state wording:

  | State | Icon | Wording | Surface |
  | --- | --- | --- | --- |
  | Private | lucide `Lock` (`size-4`) | `非公開` | `bg-elevated text-fg` (secondary `Button`, `sm`) |
  | Public | lucide `Globe` (`size-4`) | `公開中` | `bg-accent-soft text-link` (as an active tag filter) |

  The accessible name is `ログインしていない人に公開する`, and the state is
  conveyed by `aria-checked`. The icon and wording change, not only the color
  (`UI品質` "distinguishable without relying on color alone").
- Pressing sends `PUT /api/video-visibility` (`videoIds: [id]`). While sending
  it is `disabled` and the icon becomes `LoaderCircle` (`animate-spin`). The
  state changes after the response. No toast: the component's own wording
  changes, and that is the result.
- On failure, `変更できませんでした` (`role="alert"`, `AlertCircle`) appears in
  `text-sm text-danger` to the right of the line (below it under `sm`). It
  clears on the next press or when moving to another video.
- Changes made in another tab are not tracked on this screen (refetching the
  video reflects them).

### Selection bar

- Right after `タグを付ける` and `タグを外す`, before the vertical rule, sits
  `公開` (lucide `Globe` + wording + `ChevronDown`, ghost `Button`, `sm`).
  Pressing opens a `ui/Menu` upward (the current `MenuContent` takes only
  `align`, so it gains a `side` prop, as 014's `PopoverContent`) with two items:
  `公開にする` (`Globe`) and `非公開にする` (`Lock`). Two buttons are rejected:
  from `sm` up, the single line would hold four worded buttons together with the
  two tag actions, and that group would grow too long and catch the eye before
  the divider from `すべて選択`.
- The current state of the selected videos is not shown. For a selection on the
  loaded list `Video.public` tells it, but with Select all including unloaded
  pages it cannot be known, which would mean two kinds of display. Both items
  are always enabled, and a video already in the state is not an error
  ([guest-api.md §4](contracts/guest-api.md#4-switching-the-public-flag)).
- On commit it sends `PUT /api/video-visibility` and shows the toast
  `N 件を公開にしました` or `N 件を非公開にしました` (N is `applied`). The
  selection remains. The card marks in the list ("Card" below) change after the
  response.
- On failure a toast says `変更できませんでした` and the selection remains. The
  menu has already closed, so the one-line message inside a popover (the tag
  form) is not used.
- When the selection exceeds the limit (20,000, the same as tags), `公開` is
  `disabled` with the same reason as the tag actions (library-ui.md §6).

### Card

- On the owner's cards, a public video gets lucide `Globe` (`size-3`,
  `text-fg`) at the start of the badge at the thumbnail's bottom right (the
  `720P 59:11` surface), with `公開` in `sr-only`. A video without a badge (no
  quality or duration) shows only `Globe` on the same surface.
- In list view rows, the same icon sits just before the duration column.
- Guests do not see this mark: everything they see is public, so it means
  nothing.
- Public is not added to the card's primary information (thumbnail, title,
  duration, progress). The mark is one icon on the duration's surface and does
  not catch the eye before the title or thumbnail.

## Interaction states

| Control | States |
| --- | --- |
| Setup and login primary action | Normal (`bg-accent`) → hover (`bg-accent-hover`) → `disabled` (while sending, during 429, after 409; `opacity-50`). Focus is the default outer outline |
| Input | `focus:border-accent`. The border color does not change after a failure (no `aria-invalid`): the screen does not say which field is wrong, so turning both borders red means nothing. Only first-time setup validation sets `aria-invalid="true"` on the named field and points `aria-describedby` at the failure line |
| Public toggle (`role="switch"`) | Normal → hover (secondary `hover:bg-hover-wash`; while public, stays `hover:bg-accent-soft` with text `text-fg`) → `disabled` (while sending) |
| Logout item | The existing `Entry` states. `disabled` only while sending |

## Accessibility

- Each input is tied to a `label`, with `autocomplete` as in the table above
  (`UI品質`).
- The failure line and the already-configured notice are `role="alert"` and are
  read when they appear; so is the 429 wording. The connection warning has no
  `role` and is tied through `aria-describedby` on the username input and the
  primary action ("Connection warning").
- After a whole-page navigation, the destination screen's `h1` (for the list,
  the `sr-only` `ライブラリ`) receives focus under the current rules. Login
  success is not announced separately.
- `pairs` in `tokens.test.ts` gains the following, because the setup and login
  screen surface is `bg-surface` and only `fg` and `fg-muted` are checked on it
  today:
  - `["danger", "surface", 4.5]` (failure line)
  - `["warning", "surface", 4.5]` (connection warning)
  - `["link", "surface", 4.5]` (`ログインへ`)
- Focus outlines are never removed. At 360px there is no horizontal scrolling
  and no overlapping elements.
- The public toggle conveys its state through `aria-checked` on
  `role="switch"`, and the selection bar menu items differ by wording.
