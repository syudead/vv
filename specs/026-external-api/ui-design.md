# UI Design: "API tokens" section of the settings page

**Feature**: [parent Issue #493](https://github.com/syudead/vv/issues/493) ·
[plan.md](plan.md) · [contracts/token-api.md](contracts/token-api.md) ·
[research.md R-1](research.md#r-1-a-token-is-a-prefixed-256-bit-random-value-only-its-sha-256-is-stored) ·
[R-10](research.md#r-10-token-names-follow-the-same-rule-shape-as-tag-names)

The visual rules come from these documents and are not redecided here.

| Topic | Source |
| --- | --- |
| Colour, interaction states, width breakpoints | [Library UI](../../docs/design-docs/library-ui.md) |
| Role tokens | `@theme` in [`web/src/index.css`](../../web/src/index.css) |
| Pairs checked for contrast | [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts) |
| Section frame, heading, list rows and confirmation dialog of the settings page | The current [`web/src/settings/SettingsPage.tsx`](../../web/src/settings/SettingsPage.tsx) (media folders) and [`TranscodingSection.tsx`](../../web/src/settings/TranscodingSection.tsx) (video conversion) |
| Where strings live and how they are formatted | [UI strings and formatting (i18n)](../../docs/design-docs/i18n.md). The English here is a draft of intent; after implementation the catalog [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) is the source of truth. As decided in 023, the catalog is one English set, and this document adds no language mechanism (UI quality: "Japanese support is not required") |

This feature adds exactly one section to the settings page. No other screen changes. Guests cannot
enter the settings page (the 016 gate), so there is no guest fallback. The `mdm account` output
change is CLI and out of this document's scope
([contracts/token-api.md](contracts/token-api.md#mdm-account)). No new colour, radius or shadow
token is added. Only two pairs that this section checks for the first time are added to `pairs` in
`tokens.test.ts` (see "Colour" below).

## Screen boundary

- The section goes **last** on the settings page, below "Video conversion". It is last because it
  is an admin feature the owner uses only occasionally. There is no reason to read it before the
  import state or the folders (UI quality: "not more prominent than existing sections").
- The section frame (`mt-8 rounded-lg border border-border bg-surface p-4 sm:p-5`), the heading
  (`h2`, `text-base font-semibold`), the description under it (`text-sm leading-6 text-fg-muted`,
  `max-w-2xl`) and the divider below match the media folders and video conversion sections.
- The section contains, from top to bottom:
  1. The heading row and the description
  2. The create form, or the plaintext shown right after creation (one or the other)
  3. The token list
- Placing the create entry above the list differs from media folders ("Add folder" is below the
  list). The plaintext right after creation must sit close to the row it created, in the most
  prominent position of the section (UI quality: "visual hierarchy"), which is right below the
  heading.
  - Swapping the form and the plaintext in one place lets the eye pass create, plaintext, first list
    row from top to bottom once.
  - The media folders "Add folder" is a button that only opens a dialog, so its position does not
    carry the same meaning.
- The section does not depend on the import (scan) state. The media folders' disabled state
  "You can't change media folders while a scan is running" does not apply here.
- Revoke confirmation uses the same `ModalFrame` dialog as the media folders' "Remove folder" (see
  "Revoke" below). There is no other dialog, separate page or popover.

## Words

| Location | English (draft) |
| --- | --- |
| Heading | API tokens |
| Description | Tokens let external tools and MCP clients use this library as the owner. Anyone who has a token can read every video and change tags, so keep it as safe as your password. |
| Link at the end of the description | How to use the API and MCP (the published `docs/how-to/external-api.md`. Same form as "How to set up hardware encoding" in the video conversion section: `text-link underline`, `ExternalLink`, new tab) |
| Name input label | Name |
| Name input hint | What the token is for, such as the tool that will use it. |
| Create button | Create token |
| While creating | Creating… |
| Plaintext heading | Token for {name} |
| Warning below the plaintext | This is the only time the token is shown. Copy it now. After you close this, it can't be shown again; if you lose it, revoke it and create a new one. |
| Copy button | Copy token |
| Toast after copying | Copied |
| Toast when copying fails | Couldn't copy. Select the token and copy it yourself. |
| Button that closes the plaintext | Done |
| Creation time in the list | Created {date and time} |
| Last use in the list | Last used {relative time} / Never used |
| Revoke button | Revoke |
| Revoke dialog heading | Revoke this token? |
| Revoke dialog target label | Token |
| Revoke dialog warning | Tools using this token stop working right away. This can't be undone. |
| Revoke dialog button | Revoke / Revoking… |
| Toast after revoking | Revoked |
| Loading | Loading the API tokens |
| Load failure | Couldn't load the API tokens: {reason} |
| Create failure (per `reason`) | `api_token_name_empty`: Enter a name for the token. / `api_token_name_control_characters`: The name can't contain control characters. / `api_token_name_too_long`: The name can be up to {`limit`} characters. |

- Times use `formatDateTime` (for example Sep 28, 2026, 3:04 PM). The last use uses
  `formatRelative` (for example 3 minutes ago), with the `formatDateTime` time in `title`. The last
  use answers "until about when was it used" and has only minute precision (R-9), so the relative
  form fits. The creation time is absolute and helps tell tokens with the same name apart (Edge
  cases: "several tokens may share a name").
- Names and plaintexts are user data, embedded as arguments and never translated.
- The section has no "no tokens" row. With no tokens, only the description and the create form
  show; they say what the section is for and where to create (UI quality: "information density").

## Create

The create form sits below the heading divider (`pt-4`).

- **Input**: the label "Name" (`text-xs font-medium text-fg-muted`, tied with `htmlFor` on `label`)
  and the same text input as the login screen (016 ui-design "Typography": `h-9 rounded-sm border
  border-border bg-field px-3 text-sm text-fg focus:border-accent focus:outline-none`), to match the
  height of the `md` `Button`. `autocomplete="off"`. No `maxLength`: the limit is the server rule of
  R-10, and the message on overflow comes from the server's `reason`. One line of hint below the
  input in `text-xs text-fg-muted`.
- **Primary action**: primary `Button` "Create token". `disabled` while the name is empty after
  trimming. While creating, it shows a spinner (`LoaderCircle`) and "Creating…", and the input is
  `disabled` too. Enter submits (`form` submit).
- **Error line**: below the input, in place of the hint, one line in `text-sm text-danger` with
  `role="alert"`. A failure with a `reason` uses the sentence from the table above; any other failure
  uses the `errorText` sentence. The input value stays.
- **Success**: add the `token` from the 201 to the top of the list, and replace the form with the
  plaintext (see "Reveal" below). Clear the input. No toast: the plaintext itself is the result, and
  a toast draws the eye away.
- **While the list load has failed**, the form stays visible with the input and button `disabled`.
  Below the button, "You can create a token once the current tokens have loaded" shows in
  `text-xs text-fg-muted` (the same treatment as the media folders' "Add folder").

## Reveal

The plaintext right after creation appears in the form's place, instead of the form. It is the only
element in this feature that the section shows stronger than the rest.

- **Surface**: the same box as "Current location" in a media folder row (`rounded-md border
  border-control-border bg-field p-3 sm:p-4`). It stands out through position (right below the
  heading), the primary action inside and the warning line. The surface and border colours do not
  change. A loud surface colour would make the whole section noisier than the other settings
  sections (UI quality: "not more prominent than existing sections").
- **Content** (top to bottom):
  1. "Token for {name}" in `text-xs text-fg-muted`. The name uses `break-words`.
  2. The plaintext in `code` (`font-mono`, `text-sm text-fg`, `break-all`, `select-all`). It is 46
     characters (`vvt_` and 43 characters, R-1), so at 360 px it wraps to 2-3 lines. It is not
     masked. Something shown only once is not hidden again behind a "Show" action.
  3. The action row: primary `Button` "Copy token" (`Copy` icon) and secondary "Done". Copy comes
     first and is the only primary in the section at this time (action priority: create > copy).
  4. The warning line: `text-sm leading-6 text-warning`, a leading `ShieldAlert` (`size-4`, `mt-1`),
     and `border-l-2 border-warning-strong pl-3` on the left (the same form as the connection
     warning on the login screen). The sentence is in the table above.
- **Copy**: built like "Copy path" on the player screen (`navigator.clipboard`, switching to copy by
  selection on an insecure connection). Success shows the "Copied" toast; failure shows the
  "Couldn't copy. …" toast. After a failed copy the plaintext stays and can be selected by hand with
  `select-all`.
- **Close**: only "Done" closes it and returns to the form. Esc, an outside click or a timeout does
  not close it. A page reload also removes it, and it never shows again (Requirement 2).
  - No confirmation on close. The warning line already says "it can't be shown again after you
    close this" and "if you lose it, revoke it and create a new one", and recovering from a missed
    copy (revoke, then create) is cheap.
- **Why not a dialog**: `ModalFrame` closes on Esc and an outside click, and an accidental close
  loses the plaintext. Inline, only "Done" closes it.
- The list stays usable while the plaintext shows. Revoking that same token removes the plaintext
  too (no plaintext of a revoked token stays).

## List

- Below the form (or the plaintext), `mt-5`. Rows use `divide-y divide-border`, each row `py-4`
  (the same as media folder rows). The order is the API order (newest first); the UI does not
  re-sort.
- **Row**: the name and times group on the left (`min-w-0 flex-1`), "Revoke" on the right
  (`shrink-0`, aligned to the top of the row).
  - Name: `text-sm font-medium text-fg`, `break-words`. Not truncated. It is at most 100
    characters, and seeing that nothing follows is faster than reading the whole name from `title`.
  - Time line: `mt-1 text-xs text-fg-muted tabular-nums`. "Created {date and time}" and
    "Last used {relative}" (or "Never used") in `flex flex-wrap gap-x-3`, wrapping to 2 lines when
    the width is too small.
  - "Never used" uses the same colour as the times and is not emphasized. Never used is not an
    error.
- **Revoke button**: ghost `sm` `Button` with `text-danger`, the `Trash2` icon and the label
  "Revoke" (the same form as the media folders' "Remove"). While that row is revoking, the button
  is `disabled` instead of showing a spinner and "Revoking…"; the dialog button shows the progress
  (below).
- **Loading**: two `Skeleton` bars, as in media folders.
- **Load failure**: "Couldn't load the API tokens: {reason}" (`text-sm text-danger`) and a
  `Button` "Retry". The form stays visible and disabled, as in "Create" above.
- **Empty**: the list area is not shown ("Words" above).

## Revoke

- "Revoke" opens a `ModalFrame` dialog. Its content has the same structure as the media folder
  removal dialog:
  1. Target box (`rounded-md border border-control-border bg-field p-3`): the label "Token"
     (`text-xs text-fg-muted`), the name (`text-sm text-fg`, `break-words`) and below it
     "Created {date and time}" (`text-xs text-fg-muted`), to tell tokens with the same name apart
     by creation time.
  2. Warning line: `border-l-2 border-danger-strong pl-3 text-sm leading-6 text-fg-muted`.
  3. Error line (if any): `role="alert"`, `text-sm text-danger`.
  4. Bottom row: secondary "Cancel" (initial focus) and danger "Revoke". While running, the danger
     button shows a spinner and "Revoking…", and both are `disabled`.
- On success, close the dialog, remove the row from the list and show the "Revoked" toast. Focus
  moves to the next row's "Revoke", or to the name input when no row remains (the same as after a
  media folder removal).
- The initial focus is "Cancel" so that repeated Enter presses do not confirm an irreversible action
  (UI quality: "revoking cannot be undone, so it asks for confirmation").

## Interaction states

- Interaction states (hover, focus-visible, active, disabled) use the existing states of `Button`
  and the text input. This section creates no new state.
- While creating, the form input and button are `disabled`. While revoking, both dialog buttons are
  `disabled`. The list's "Revoke" stays usable while creating (create and revoke are independent
  operations on different tokens).
- A 401 received while rendering as the owner follows the 016 gate rule (reload the page once). The
  reload removes the plaintext.

## Colour

No new token. These pairs used here are already in `pairs` of `tokens.test.ts`:

| Use | Pairs |
| --- | --- |
| Body text | `fg` on `surface`, `field`, `elevated`; `fg-muted` on `surface`, `elevated` |
| Error line | `danger` on `surface`, `elevated` |
| Box border | `control-border` on `surface`, `field`, `elevated` |
| Primary action | `accent-fg` on `accent` |

Text on the plaintext box and on the revoke dialog's target box (`bg-field`) uses two pairs not yet
in `pairs`. The implementation adds them to `pairs` (both 4.5 or higher).

- `fg-muted` on `field`: "Token for {name}", and "Token" and "Created {date and time}" in the revoke
  dialog.
- `warning` on `field`: the warning line of the plaintext.

`border-warning-strong` and `border-danger-strong` are left rules that 016 and the settings page
already use for the same purpose. The sentence and the icon carry the meaning, not the line, so they
are not added as pairs.

## Responsive layout

The widths checked are 360 px, 768 px and 1280 px. The section fits inside the settings page's
`max-w-4xl` and does not widen at 1280 px.

- **360 px**:
  - Form: label, input (`w-full`), hint and button (`w-full`) stacked vertically.
  - Plaintext: wraps to 2-3 lines and stays inside the box. The action row puts "Copy token" and
    "Done" on one line and wraps when they do not fit. The warning line wraps along the left rule.
  - List row: "Revoke" stays at the top right even when the name wraps. The time line becomes 2
    lines, "Created …" and "Last used …".
  - Revoke dialog: the existing narrow form of `ModalFrame`.
- **768 px and wider**:
  - Form: label and hint unchanged; the input (`max-w-sm`) and "Create token" on one line.
  - List row: fits in 2 lines, one for the name and one for the times (a 100-character name may take
    2 lines).
  - Plaintext: 1-2 lines.
- **1280 px**: same as 768 px. The section is as wide as the other sections, and the plaintext box
  spans the full section width (not stopped at `max-w-2xl`: a plaintext on one line is easier to
  check after copying).

## Review criteria

Judge the following by eye at 360 px, 768 px and 1280 px. They map to the five UI quality aspects:
visual hierarchy, information density, spacing rhythm, typography and action priority.

1. **Hierarchy**: on opening, the section reads heading, create entry, list in that order. Right
   after creation, the plaintext box and "Copy token" are the strongest elements in the section and
   catch the eye before the heading. At other times no element in the section is stronger than the
   rest.
2. **Once only**: after create, "Done" and a reload, no part of the plaintext (a string containing
   `vvt_`) appears anywhere in the section, and the list keeps only names and times.
3. **Density**: a section with 3 tokens is about as tall as the media folders section with 3
   folders. One token fits in 2 lines at 768 px and wider, and the name is stronger than the times.
   With no tokens, the section has only the heading, the description and the form, with no "none"
   row and no empty frame.
4. **Spacing rhythm**: the spacing above the section, the frame, the divider below the heading, and
   the row dividers and vertical padding cannot be told apart from the media folders section. The
   gap between the form and the list, and between the elements inside the plaintext box, is neither
   tighter nor looser than the other gaps in the section.
5. **Typography**: only the plaintext is monospace; everything else uses the same font and size as
   the other sections. At 360 px, the plaintext and a 100-character name without spaces do not
   overflow the box or the row. Time digits do not shift between rows (`tabular-nums`).
6. **Action priority**: the section always has exactly one primary button ("Create token" or
   "Copy token"). "Revoke" is a ghost button in the danger colour, weaker than primary. Pressing it
   always opens the dialog, and the dialog's initial focus is on "Cancel".
7. **Not too prominent**: on the whole settings page, this section draws less attention than the
   import state or media folders. The accent colour appears only on the primary button and focus
   outlines.
8. **No overlap**: the plaintext and a toast, and the revoke dialog and a toast, never overlap and
   stay readable at every width.
