# Components rules

Every control, overlay and mark on a screen is one of the components in
`web/src/ui`, read through its registry item (`npx shadcn view
./registry/r/<item>.json` from `web/`). Each is the shadcn/ui component on the
Radix base, with its upstream structure and variants, dressed only through the
tokens in [foundations.md](foundations.md). A screen composes them; it does not
restyle them with its own colours, radii or heights. The reasons are in
[design-system.md, Components](../../../docs/design-docs/design-system.md#components).

The PascalCase files beside them (`Popover.tsx`, `Menu.tsx`, `ModalFrame.tsx`,
`Toast.tsx`, `Chip.tsx`, `Tooltip.tsx`, `Tabs.tsx`, `Skeleton.tsx`, and
`videoList/FavoriteToggle.tsx`) are the components these replace. They stay
only for screens that have not moved yet; new code never imports them.
`Popover`, `Tooltip`, `Tabs` and `Skeleton` live in `web/src/ui/next/` until
the old file of the same name is gone, because two names differing only in
case cannot share a folder.

## Overlays and feedback

### Dialog

A modal window for a task that needs the viewer's full attention: a form
(create, merge, edit synonyms) or a choice from a long list. Compose
`DialogHeader` (`DialogTitle`, `DialogDescription`), the body, and a
`DialogFooter` with `Cancel` and at most one primary action. The window is at
most `max-w-lg` and keeps a 16px margin on narrow screens.

Do not use it to confirm a destructive action (use `AlertDialog`), to show a
short hint (use `Tooltip`) or for options next to a control (use `Popover`).

### AlertDialog

Confirms an action that cannot be undone, such as deleting or rejecting a tag.
Say what happens in `AlertDialogTitle` and `AlertDialogDescription`, put
`AlertDialogCancel` first and name `AlertDialogAction` by its verb with
`variant="destructive"`. It does not close on an outside click.

Do not use it for a reversible action, which needs no confirmation, or for a
form (use `Dialog`).

### Popover

A non-modal panel anchored to its trigger, for a few options or a short form
that belongs to that control: view and sort, adding a tag to the selection,
the scan summary. Default width `w-popover`; pass `container` when the
trigger sits in a full-screen element. Combine with `PopoverHeader` for a
title.

Do not use it for a list of actions (use `DropdownMenu`) or for a task that
needs the whole screen (use `Dialog`).

### DropdownMenu

A list of actions or one choice from a few, opened from a button: visibility,
favorites, sort order. Use `DropdownMenuItem` for actions,
`DropdownMenuRadioGroup` with `DropdownMenuRadioItem` for one choice and
`DropdownMenuCheckboxItem` for toggles. A destructive item uses
`variant="destructive"` and sits after a `DropdownMenuSeparator`.

Do not put inputs in it (use `Popover`) or use it for navigation between
screens (use `Sidebar` or links).

### Tooltip

Names an icon-only control or adds a short hint, shown on hover and keyboard
focus. One `TooltipProvider` wraps the app. The text is one short line; a
`Kbd` may follow it.

Do not put interactive content or anything the viewer must read to act in it;
touch screens never show it.

### Tabs

Switches between views of the same content in place, such as the tag admin
lists. `TabsList` with `variant="default"` is the filled segment;
`variant="line"` is the underlined row for page-level tabs. A count follows
the label in `text-muted-foreground`.

Do not use it to change a setting (use `ToggleGroup`) or to move to another
screen (use links).

### Sonner

Short, passive notices of a finished action ("Added 3 videos to favorites").
Call `toast()` from `sonner` under one `Toaster` for the app. A notice is one
line and needs no action.

Until every screen moves, the app's notices still go through `useToast` from
`ui/Toast`, which also queues notices one at a time on the video page. The app
switches to `Toaster` in one change, so keep calling `useToast` in existing
screens.

Do not use it for errors the viewer has to fix (use `Alert` near the cause) or
for anything that must stay on screen.

### Badge

A small label: a tag chip, a count, a status. `secondary` for tags, `soft` for
a selected or active chip, `default` for a count that needs the brand colour,
and `destructive`, `warning`, `success` for status, always with a word and an
icon. With `asChild` it can wrap a link; a removable chip puts its `×` button
inside.

Do not use it as a button for a primary action or for long text; the label is
one short phrase.

### Skeleton

The final layout's shapes while content loads: cards, list rows, a line of
text. Give it the size of the content it stands for.

Do not cover a whole page with a spinner instead, or use it for content that
loads faster than the eye notices.

### Progress

How far a known amount of work has come: a scan's videos, the part of a video
watched. Pass `value` and `max`, and `aria-label`. Without a `value` it is
indeterminate.

Do not use it for a wait with no measure; use `Spinner` for that.

### Spinner

A short wait with no measure: loading more items at the end of a list, a
pressed action that is still running. It carries the accessible name
`Loading`.

Do not use it in place of a `Skeleton` for the first load of a list.

### Alert

A message inside a section: an error with `variant="destructive"`, a warning
(stalled playback) with `warning`, done with `success`, or neutral guidance
(autoplay is off) with the default. Start with an icon, then `AlertTitle` and
`AlertDescription`; put a `Retry` in `AlertAction` when retrying can help.

Do not use it for a passing notice (use `Sonner`) or to fill an empty list
(use `Empty`).

### Empty

The empty state of a list or section: an icon in `EmptyMedia`
(`variant="icon"`), one line in `EmptyTitle` saying why it is empty, and in
`EmptyContent` the action that fills it when the viewer can take it.

Do not use it while loading (use `Skeleton`) or after a failure (use `Alert`).

### Separator

A hairline between groups of controls or sections. Horizontal by default,
`orientation="vertical"` inside a row.

Do not add separators where spacing already separates the groups.

### Kbd

A key or a search operator in running text or a tooltip (`tag:`, `-`). Group
several with `KbdGroup`.

Do not use it for anything the viewer cannot type.

### Breadcrumb

The path to the current folder. `BreadcrumbLink` (with `asChild` around a
router link) for each ancestor, `BreadcrumbPage` for the current one,
`BreadcrumbSeparator` between them, `BreadcrumbEllipsis` when the middle is
collapsed.

Do not use it for a flat set of filters or for steps of a task.

### Sidebar

The app's main navigation, from `SidebarProvider` down. vv uses three states:
expanded at wide widths, `collapsible="icon"` (the rail, labels in tooltips)
when collapsed, and a `Sheet` drawer below 640px (`use-mobile`). Entries are
`SidebarMenuButton` with `asChild` around a router link, `isActive` on the
current screen and `tooltip` set to the label; account entries go in
`SidebarFooter`. Pass `open` and `onOpenChange` to keep the viewer's choice.

Do not add a second sidebar or put page controls in it; they belong in the
toolbar.

### Sheet

A panel from the screen's edge. In vv it is only the sidebar's narrow-width
drawer; `Sidebar` renders it.

Do not use it for forms or details; use `Dialog`.

## vv components

### VideoThumbnail

The 16:9 thumbnail frame shared by video, group and folder cards and list
rows. Put the image (or `VideoThumbnailImage`, or a hover preview) as
children, then the overlays it needs: `VideoThumbnailDuration` bottom right
(a public mark may come first), `VideoThumbnailProgress` on the bottom edge
for the share watched, `VideoThumbnailMark` in a top corner for the selection
check (`top-start`) and `FavoriteToggle` (`top-end`), and
`VideoThumbnailNotice` over the whole frame when the video cannot play.
`selected` rings the frame in `primary`.

Do not draw a thumbnail with its own frame, badge or progress bar, and do not
put text other than the duration and the notice on the image.

### FavoriteToggle

The favorite heart, which is both the mark and the toggle. `variant="card"`
sits in a `VideoThumbnailMark` on a card, `row` in a list row and `page` in the
video page's action group. The heart is filled in `favorite` when on; when off
it shows only on hover and focus of its card or row. Shown to the owner only.

Do not use the `favorite` colour or a heart for anything else.

### TentativeMark

The dashed circle after a tentative tag's name, in a `Badge` (`size="chip"`)
or an admin row (`size="row"`). It is decorative; the element around it says
"tentative" to screen readers.

Do not use it for anything other than tentative tags.

### ScrubPreview

The card scrub: a band at the bottom fifth of a card's thumbnail that shows
the frame under the pointer (`useScrubPreview`, `ScrubBand`, `ScrubFrame`).
It goes inside a `VideoThumbnail`.

Do not confuse it with the video page's seek preview over the player's
progress bar, which is a `special` exception in `index.css`.

### ThumbnailBackdrop

A blurred copy of a portrait thumbnail behind it, so a 9:16 video fills its
16:9 frame. Put it first inside the frame, with the image above it.

Do not use it for landscape images, which fill the frame already.

### BrandHomeLink

The VVMDM logo in the top bar, linking to the library: the symbol below the
`sm` width and the wordmark above it. One per screen.

Do not use the logo images anywhere else.
