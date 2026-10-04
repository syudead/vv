# UI design: "API tokens" section on the settings page

**Feature**: [parent Issue #493](https://github.com/syudead/vv/issues/493) ·
[plan.md](plan.md) · [contracts/token-api.md](contracts/token-api.md) ·
[research.md R-1](research.md#r-1-tokens-are-prefixed-256-bit-random-values-and-only-the-sha-256-is-stored) ·
[R-10](research.md#r-10-token-names-follow-the-same-rules-as-tag-names)

Sources (this document does not re-decide what they define):

| Topic | Source |
| --- | --- |
| Colours, interaction states, width breakpoints | [Library UI](../../docs/design-docs/library-ui.md) |
| Role tokens | `@theme` in [`web/src/index.css`](../../web/src/index.css) |
| Contrast pairs under test | [`web/src/theme/tokens.test.ts`](../../web/src/theme/tokens.test.ts) |
| Settings section frame, heading, list rows, confirmation dialog | The current [`web/src/settings/SettingsPage.tsx`](../../web/src/settings/SettingsPage.tsx) (Media folders) and [`TranscodingSection.tsx`](../../web/src/settings/TranscodingSection.tsx) (Video conversion) |
| Where screen text lives and its format | [Screen text and formatting (i18n)](../../docs/design-docs/i18n.md). The English in this document is a proposal showing intent; after implementation the catalogue [`web/src/i18n/en.ts`](../../web/src/i18n/en.ts) is the source of truth. As decided in 023, the catalogue is a single English set, and this document adds no language mechanism (`UI品質`: Japanese support is not required). |

This feature adds exactly one section to the settings page; no other screen changes. Guests cannot
enter the settings page (the 016 gate), so there is no guest fallback. The change to the
`mdm account` output is CLI and outside this document
([contracts/token-api.md](contracts/token-api.md#mdm-account)). No new colour, radius or shadow token
is added. Only two pairs that this section is the first to test are added to `pairs` in
`tokens.test.ts` (see "Colour" below).

## Screen boundary

- The section goes **last** on the settings page, below "Video conversion". The section frame
  (`mt-8 rounded-lg border border-border bg-surface p-4 sm:p-5`), the heading (`h2`,
  `text-base font-semibold`), the description under it (`text-sm leading-6 text-fg-muted`,
  `max-w-2xl`) and the divider below are the same as the Media folders and Video conversion
  sections. It goes last because it is an admin feature the owner uses only occasionally, and there is
  no reason to have it read before the import status or the folders (`UI品質`: do not make it look
  more prominent than the existing sections).
- Inside the section, from top to bottom:
  1. Heading row and description
  2. The create form, or the plaintext shown right after creation (one or the other)
  3. The token list
- Placing the create entry above the list differs from Media folders ("Add folder" is below the
  list). The plaintext right after creation must appear right next to the row it created, in the most
  prominent position in the section (`UI品質`: visual hierarchy), and that position is directly under
  the heading. Swapping the form and the plaintext in the same place lets the eye pass create →
  plaintext → first list row from top to bottom in one go. "Add folder" in Media folders is a button
  that only opens a dialog, so its position does not carry the same meaning.
- The section is not affected by the import (scan) status. Media folders' "cannot change during a
  scan" disabling does not apply to this section.
- Revoke confirmation uses the same `ModalFrame` dialog as Media folders' folder removal (see "Revoke"
  below). There is no other dialog, separate page or popover.

## Words

| Place | Text (proposed) |
| --- | --- |
| Heading | API tokens |
| Description | Tokens let external tools and MCP clients use this library as the owner. Anyone who has a token can read every video and change tags, so keep it as safe as your password. |
| Link at the end of the description | How to use the API and MCP (the published `docs/how-to/external-api.md`. Same form as "How to set up hardware encoding" in the Video conversion section: `text-link underline`, `ExternalLink`, new tab) |
| Name input label | Name |
| Name input hint | What the token is for, such as the tool that will use it. |
| Create button | Create token |
| While creating | Creating… |
| Plaintext heading | Token for {name} |
| Notice under the plaintext | This is the only time the token is shown. Copy it now. After you close this, it can't be shown again; if you lose it, revoke it and create a new one. |
| Copy button | Copy token |
| Toast after copying | Copied |
| Toast when copying failed | Couldn't copy. Select the token and copy it yourself. |
| Button that closes the plaintext | Done |
| Created date in the list | Created {date} |
| Last used in the list | Last used {relative time} / Never used |
| Revoke button | Revoke |
| Revoke dialog heading | Revoke this token? |
| Revoke dialog target label | Token |
| Revoke dialog notice | Tools using this token stop working right away. This can't be undone. |
| Revoke dialog button | Revoke / Revoking… |
| Toast after revoking | Revoked |
| Loading | Loading the API tokens |
| Load failure | Couldn't load the API tokens: {reason} |
| Create failure (per `reason`) | `api_token_name_empty`: Enter a name for the token. / `api_token_name_control_characters`: The name can't contain control characters. / `api_token_name_too_long`: The name can be up to {`limit`} characters. |

- Dates use `formatDateTime` (for example, Sep 28, 2026, 3:04 PM). Last used uses `formatRelative`
  (for example, 3 minutes ago) with the `formatDateTime` date in `title`. Last used answers "until
  about when was it used" and has only minute precision (R-9), so the relative form fits. The created
  date is absolute and helps tell apart tokens with the same name (Edge Cases: several tokens can
  share a name).
- The name and the plaintext are user data, embedded as arguments and not translated.
- The section has no line saying "there are no tokens". With no tokens, only the description and the
  create form show, and they state what the section is for and where to create one (`UI品質`:
  information density).

## Create

The create form sits below the heading divider (`pt-4`).

- **Input**: label "Name" (`text-xs font-medium text-fg-muted`, tied with `htmlFor` on `label`) and
  the same text input as the login screen (016 ui-design "Typography": `h-9 rounded-sm border
  border-border bg-field px-3 text-sm text-fg focus:border-accent focus:outline-none`), to match the
  height of the `md` `Button`. `autocomplete="off"`, and no `maxLength` (the limit is the server rule
  in R-10, and the message when it is exceeded is built from the server's `reason`). One line of hint
  under the input in `text-xs text-fg-muted`.
- **Primary action**: primary `Button` "Create token". `disabled` while the name, trimmed of
  surrounding whitespace, is empty. While creating, it shows a spinner (`LoaderCircle`) and
  "Creating…", and the input is `disabled` too. Enter submits (`form` submit).
- **Failure line**: below the input, in place of the hint, one line in `text-sm text-danger` with
  `role="alert"`. A failure with a `reason` uses the sentence from the table above; any other failure
  uses the `errorText` sentence. The input value is kept.
- **Success**: the `token` from the 201 is added to the top of the list, and the form is replaced by
  the plaintext display ("Reveal" below). The input value is cleared. No toast: the plaintext display
  is itself the result, and a toast would pull the eye away.
- **While the list has failed to load**, the form stays visible with the input and button
  `disabled`, and "You can create a token once the current tokens have loaded" shows below the button
  in `text-xs text-fg-muted` (the same treatment as "Add folder" in Media folders).

## Reveal

The plaintext right after creation shows in the same place as the form, instead of the form. It is
the only element in this feature that is shown more strongly than the rest of the section.

- **Surface**: the same box as "Current location" in a Media folders row (`rounded-md border
  border-control-border bg-field p-3 sm:p-4`). It stands out through its position (directly under the
  heading), the primary action inside it and the notice line; the surface and border colours do not
  change. Adding a surface in a prominent colour would make the whole section noisier than the other
  sections on the settings page (`UI品質`: do not make it look more prominent than the existing
  sections).
- **Contents** (top to bottom):
  1. "Token for {name}" in `text-xs text-fg-muted`. The name is `break-words`.
  2. The plaintext in `code` (`font-mono`, `text-sm text-fg`, `break-all`, `select-all`). It is 46
     characters (`vvt_` plus 43, R-1), so at 360px it wraps to 2–3 lines. It is not masked: something
     visible only once is not hidden further behind a "show" action.
  3. Action row: primary `Button` "Copy token" (with the `Copy` icon) and secondary "Done". Copy comes
     first and is the only primary in the section at this moment (action priority: create > copy).
  4. Notice line: `text-sm leading-6 text-warning`, led by `ShieldAlert` (`size-4`, `mt-1`), with
     `border-l-2 border-warning-strong pl-3` on the left (the same form as the connection warning on
     the login screen). The text is in the table above.
- **Copy**: built like "Copy path" on the playback screen (`navigator.clipboard`, falling back to copy
  by selection on an insecure connection). Success shows the "Copied" toast, failure the "Couldn't
  copy. …" toast. If copying fails, the plaintext stays and can be selected by hand with `select-all`.
- **Close**: only "Done" closes it, returning to the form. Esc, a click outside and a timeout do not
  close it. A page reload also removes it, and it never shows again (requirement 2). No confirmation
  on close: the notice line already says "it can't be shown again after you close this" and "if you
  lose it, revoke it and create a new one", and recovering from a forgotten copy (revoke → create) is
  cheap.
- **Why not a dialog**: `ModalFrame` closes on Esc and on a click outside, so an accidental close
  loses the plaintext. In place, only "Done" closes it.
- The list stays usable while the plaintext shows. Revoking the same token also removes the
  plaintext display (the plaintext of a revoked token does not linger).

## List

- Below the form (or the plaintext display), `mt-5`. Rows are `divide-y divide-border`, each `py-4`
  (the same as Media folders rows). The order is the API's order (newest first), not re-sorted on
  screen.
- **Row**: on the left, the name and dates block (`min-w-0 flex-1`); on the right, "Revoke"
  (`shrink-0`, aligned to the top of the row).
  - Name: `text-sm font-medium text-fg`, `break-words`. Not truncated: it is at most 100 characters,
    and seeing that nothing follows is faster than reading the whole name through `title`.
  - Date line: `mt-1 text-xs text-fg-muted tabular-nums`. "Created {date}" and "Last used {relative}"
    (or "Never used") side by side with `flex flex-wrap gap-x-3`; at widths where they do not fit they
    take two lines.
  - "Never used" is written in the same colour as the dates and is not emphasised. Unused is not an
    error.
- **Revoke button**: ghost `Button`, `sm`, `text-danger`, `Trash2` icon, label "Revoke" (the same form
  as "Remove" in Media folders). While a row is being revoked, instead of a spinner and "Revoking…" the
  row button is `disabled`, and the dialog button shows progress (below).
- **Loading**: two `Skeleton` bars, as in Media folders.
- **Load failure**: "Couldn't load the API tokens: {reason}" (`text-sm text-danger`) and a `Button`
  "Retry". The form stays visible and disabled as described in "Create".
- **Empty**: the list area is not shown (see "Words").

## Revoke

- Pressing "Revoke" opens a `ModalFrame` dialog. Its contents are laid out like the Media folders
  removal dialog:
  1. Target box (`rounded-md border border-control-border bg-field p-3`): label "Token"
     (`text-xs text-fg-muted`), the name (`text-sm text-fg`, `break-words`), and below it
     "Created {date}" (`text-xs text-fg-muted`), to tell apart tokens with the same name by creation
     date.
  2. Notice line: `border-l-2 border-danger-strong pl-3 text-sm leading-6 text-fg-muted`.
  3. Failure line (if any): `role="alert"`, `text-sm text-danger`.
  4. Footer: secondary "Cancel" (initial focus) and danger "Revoke". While running, the danger button
     shows a spinner and "Revoking…", and both are `disabled`.
- On success the dialog closes, the row leaves the list, and the "Revoked" toast shows. Focus moves to
  the next row's "Revoke", or to the name input if no rows remain (the same as after removing a media
  folder).
- The dialog's initial focus is on "Cancel" so that repeated Enter presses do not confirm an
  irreversible action (`UI品質`: revoking cannot be undone, so it asks for confirmation).

## Interaction states

- Interaction states (hover, focus-visible, active, disabled) use the existing states of `Button` and
  the text input; this section creates no new state.
- While creating, the form input and button are `disabled`; while revoking, both dialog buttons are
  `disabled`. A list row's "Revoke" stays pressable while creating (creating and revoking are
  independent operations on different tokens).
- A 401 received while rendering as the owner follows the 016 gate rule (reload the page once). The
  reload removes the plaintext display.

## Colour

No new token is added. Of the pairs used, these are already in `pairs` in `tokens.test.ts`: body text
(`fg` on `surface`, `field` and `elevated`; `fg-muted` on `surface` and `elevated`), the failure line
(`danger` on `surface` and `elevated`), box borders (`control-border` on `surface`, `field` and
`elevated`), and the primary action (`accent-fg` on `accent`).

Text on the `bg-field` box of the plaintext display and of the revoke dialog's target uses two pairs
not yet in `pairs`; the implementation adds them (both at 4.5 or more).

| Pair | Used by |
| --- | --- |
| `fg-muted` on `field` | "Token for {name}", the revoke dialog's "Token" and "Created {date}" |
| `warning` on `field` | The notice line of the plaintext display |

`border-warning-strong` and `border-danger-strong` are left-hand rules that 016 and the settings page
already use for the same purpose; the sentence and the icon carry the meaning, not the line, so they
are not added as pairs.

## Responsive layout

The widths judged are 360px, 768px and 1280px. The section stays within the settings page's
`max-w-4xl` and does not widen at 1280px.

| Width | Form | Plaintext display | List row | Revoke dialog |
| --- | --- | --- | --- | --- |
| 360px | Stacked: label, input (`w-full`), hint, button (`w-full`). | The plaintext wraps to 2–3 lines without overflowing the box. "Copy token" and "Done" share one row and wrap if they do not fit. The notice line wraps along the left rule. | "Revoke" stays at the top right even when the name wraps. The date line becomes two lines, "Created …" and "Last used …". | The existing narrow form of `ModalFrame`. |
| 768px and up | Label and hint unchanged; the input (`max-w-sm`) and "Create token" on one row. | The plaintext takes 1–2 lines. | Fits in two lines: one for the name, one for the dates (a 100-character name may take two). | — |
| 1280px | Same as 768px. | The section is as wide as the other sections, and the plaintext box spans the full section width (not stopped at `max-w-2xl`: a plaintext readable on one line is easier to check after copying). | Same as 768px. | — |

## Review criteria

Judge each of the following by eye at 360px, 768px and 1280px. They map to the five `UI品質` aspects
(visual hierarchy, information density, spacing rhythm, typography, action priority).

1. **Hierarchy**: on opening the section, it reads heading → create entry → list. Right after
   creation, the plaintext box and "Copy token" are the strongest elements in the section and catch
   the eye before the heading. At any other time, no element in the section is stronger than the
   rest.
2. **Shown once**: after create → "Done" → reload, no part of the plaintext (a string containing
   `vvt_`) is anywhere in the section, and the list holds only the name and dates.
3. **Density**: a section with 3 tokens is about as tall as the Media folders section with 3 folders.
   One token fits in two lines at 768px and up, and the name is stronger than the dates. With no
   tokens, the section has only the heading, description and form, with no "none" line and no empty
   frame.
4. **Spacing rhythm**: the space above the section, the frame, the divider under the heading, the row
   dividers and the vertical row padding are indistinguishable from the Media folders section. The gap
   between the form and the list, and between elements in the plaintext box, is neither tighter nor
   looser than the other gaps in the section.
5. **Typography**: only the plaintext is monospace; everything else uses the same typeface and size
   as the other sections. At 360px, the plaintext and a 100-character name without spaces do not
   overflow the box or row. Date digits do not shift between rows (`tabular-nums`).
6. **Action priority**: the section always has exactly one primary button ("Create token" or "Copy
   token"). "Revoke" is a ghost button in the danger colour, weaker than the primary; pressing it
   always opens the dialog, and the dialog's initial focus is on "Cancel".
7. **Not too prominent**: looking at the whole settings page, this section draws less attention than
   the import status or Media folders. The accent colour appears only on the primary button and the
   focus outline.
8. **No overlap**: the plaintext display and a toast, and the revoke dialog and a toast, do not overlap
   and stay readable at every width.
