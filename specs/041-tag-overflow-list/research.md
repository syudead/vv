# Research: Hidden tags behind a card's `+N`

Inherited decisions: the tag row, its `+N` chip and how the visible count is
measured
([specs/014-video-tags/ui-design.md, Overflow](../014-video-tags/ui-design.md#overflow)
and
[Card structure and pressing](../014-video-tags/ui-design.md#card-structure-and-pressing));
the folder-derived and tentative chip forms
([017 UI design, Folder-derived tag chip](../017-folder-groups/ui-design.md#folder-derived-tag-chip),
[031 UI design, Tentative mark](../031-tentative-tags/ui-design.md#tentative-mark)); the
design system's floating layers and checks
([docs/design-docs/design-system.md](../../docs/design-docs/design-system.md),
[web/registry/rules/components.md, Popover](../../web/registry/rules/components.md#popover));
the mouse-only 400 ms rest that starts a card's hover preview
([010 UI design, Interaction](../010-hover-video-preview/ui-design.md#interaction)).
This file records only the decisions this feature adds.

## R-1: Resting the mouse on `+N` opens the same `Popover` that pressing opens

**Decision**: `+N` keeps its `Popover`, and a hook beside `CardTagRow` drives
its `open` state from the pointer: a pointer with `pointerType === "mouse"`
that rests on `+N` for 400 ms opens it, and it closes 200 ms after the pointer
has left both `+N` and the list, unless the pointer comes back in that time.
Pressing `+N` (touch, pen, mouse click, `Enter` or `Space`) toggles it as today.
Focus alone does not open it. A list opened by hover takes no keyboard focus
and returns none when it closes; a list opened by pressing focuses its first
chip and returns focus to `+N`, as today.

| Option | Touch and keyboard | Esc, outside press, position, viewport | Layers over the card | Verdict |
| --- | --- | --- | --- | --- |
| **`Popover` with `open` driven by a hover-intent hook** | Pressing keeps working through the same trigger | Radix Popover, already in place | One | Chosen |
| Radix `HoverCard` as a new registry item | Ignores touch pointers and opens on focus, not on a press, so it fails requirement 1 and acceptance criterion 4 on its own | Radix HoverCard | One | Rejected: needs a second layer for touch and keyboard |
| `HoverCard` for the mouse and `Popover` for pressing, on one trigger | Covered | Two Radix layers to keep in step | Two | Rejected: two copies of the chip list, and two layers can open at once |
| CSS `:hover` on a sibling list | No touch or keyboard path | None: no collision handling, no Esc, no outside press | One | Rejected: fails requirements 1 and 3 |

**Rationale**: Radix Popover already gives the list its position, collision
handling, `Esc`, outside press and focus return, so hover only has to set the
state it already reads. 400 ms is the rest the repository uses before a
hover reveals something (the Tooltip and the hover preview), so a pointer
crossing the row does not open the list (acceptance criterion 3); the 200 ms
close delay covers the 6 px gap between `+N` and the list (requirement 2). A
hover-opened layer that took focus would pull the keyboard focus to wherever
the mouse happens to be.

## R-2: The list is a wrapping row of chips that scrolls inside the viewport

**Decision**: The list lays the hidden tags out as wrapping chips, in the
chips' order, with the row's `gap-1` between them. `PopoverContent` caps its
height to Radix's `--radix-popover-content-available-height`, as `Select` and
`DropdownMenu` already do, and the list scrolls vertically inside it. Radix's
collision handling (`collisionPadding` 8) keeps the layer inside the viewport
horizontally and places it above `+N` when there is no room below. The list's
width is one of the existing popover width steps; `ui-design.md` chooses it.

| Option | Requirement 3: every tag reachable | Requirement 4: few eye movements | Stays on the card | Verdict |
| --- | --- | --- | --- | --- |
| **Wrapping chips, height capped to the available height, scrolling inside** | Yes: the layer never leaves the viewport and the rest scrolls | Yes: about 3 chips per line at the popover width | Yes | Chosen |
| Today's one-per-line list, with scrolling added | Yes | No: 30 tags are 30 lines | Yes | Rejected: fails requirement 4 |
| A `Dialog` or `Sheet` listing the tags | Yes | Yes | No: the card disappears behind the overlay, and three hidden tags get a full-screen layer on a phone | Rejected: the parent Issue asks to check the tags without leaving the card |
| Expanding the card's row in place to show every tag | Yes | Yes | Yes | Rejected: the card grows and every card after it moves, and the fit measurement of 014 would be undone |

**Rationale**: The two faults the parent Issue names are a layer that leaves
the viewport and a column that cannot be scanned; capping the height and
wrapping the chips remove both without a new component. The cap goes into
`PopoverContent` rather than this one list because every popover is expected
to stay inside the viewport, and the floating layers that already read Radix's
available height are `special` entries in `web/design-exceptions.js`, so the
class has a home where the checks accept it.

## R-3: Chips in the list never truncate; a long name wraps inside its chip

**Decision**: A chip in the list is as wide as its name, up to the list's
width. A name wider than that wraps onto further lines inside the chip
(`whitespace-normal`, `break-all`), and the chip grows in height for those
lines only; a name that fits keeps the row chip's height and text. The folder
mark and the tentative mark stay with the first line. The chips in the card's
row are unchanged and keep truncating ([014 UI design, Tag row](../014-video-tags/ui-design.md#tag-row)).

| Option | Requirement 5: full name without a mouse | Chip size as in the row | Verdict |
| --- | --- | --- | --- |
| **No truncation; a too-wide name wraps inside its chip** | Yes, on every device | Yes for every name that fits the list width; taller only for the rest | Chosen |
| Truncate, full name in `title` and a `Tooltip` | No: touch screens never show either | Yes | Rejected: fails requirement 5 |
| Truncate, full name on a long press | No: an invisible gesture, and the press already filters | Yes | Rejected: fails requirement 5 for anyone who does not find the gesture |

**Rationale**: The list is the one place where every tag of the card is laid
out at once, so it is where a name can be read in full; nothing else is
competing for the width. Wrapping is reserved for names wider than the list,
so the chips stay the size of the row's for every ordinary name.

## R-4: The list is derived from the current tags, never copied when it opens

**Decision**: The list renders the tags after the visible count from the same
`tags` prop and measured count the row renders from; nothing is copied into
state when the list opens. When the hidden set becomes empty, or selection
starts, `+N` and its list unmount together.

| Option | Tags change while open | Zoom or width changes the hidden count while open | Verdict |
| --- | --- | --- | --- |
| **Derive on every render** | The list shows the new set | The list shows the new set, or closes with `+N` when nothing is hidden | Chosen |
| Copy the hidden set into state when the list opens | The list keeps the old set | The list keeps the old set | Rejected: the parent Issue's edge cases forbid showing an old set |

**Rationale**: The row already measures and re-renders on every tag, zoom and
width change through the list's one `ResizeObserver`
([014 UI design, Overflow](../014-video-tags/ui-design.md#overflow)); reading
the same values is what keeps the list and the row in step.
