# vv registry index

The vv registry lists the components, tokens, rules and page patterns that every
screen in `web/src` is built from. Read an item from `web/` with
`npx shadcn view ./registry/r/<item>.json`; the design decisions are in
[design-system.md](../../docs/design-docs/design-system.md).

| Item                 | Tier               | When to use                                                               |
| -------------------- | ------------------ | ------------------------------------------------------------------------- |
| `vv`                 | Index              | This list                                                                 |
| `brand-home-link`    | Existing component | `web/src/ui/BrandHomeLink.tsx`; rules arrive with the components tier     |
| `button`             | Existing component | `web/src/ui/Button.tsx`; rules arrive with the components tier            |
| `checkbox`           | Existing component | `web/src/ui/Checkbox.tsx`; rules arrive with the components tier          |
| `chip`               | Existing component | `web/src/ui/Chip.tsx`; rules arrive with the components tier              |
| `combobox`           | Existing component | `web/src/ui/Combobox.tsx`; rules arrive with the components tier          |
| `filter-chip`        | Existing component | `web/src/ui/FilterChip.tsx`; rules arrive with the components tier        |
| `icon-button`        | Existing component | `web/src/ui/IconButton.tsx`; rules arrive with the components tier        |
| `menu`               | Existing component | `web/src/ui/Menu.tsx`; rules arrive with the components tier              |
| `modal-frame`        | Existing component | `web/src/ui/ModalFrame.tsx`; rules arrive with the components tier        |
| `popover`            | Existing component | `web/src/ui/Popover.tsx`; rules arrive with the components tier           |
| `scrub-preview`      | Existing component | `web/src/ui/ScrubPreview.tsx`; rules arrive with the components tier      |
| `segmented-control`  | Existing component | `web/src/ui/SegmentedControl.tsx`; rules arrive with the components tier  |
| `skeleton`           | Existing component | `web/src/ui/Skeleton.tsx`; rules arrive with the components tier          |
| `tabs`               | Existing component | `web/src/ui/Tabs.tsx`; rules arrive with the components tier              |
| `tentative-mark`     | Existing component | `web/src/ui/TentativeMark.tsx`; rules arrive with the components tier     |
| `thumbnail-backdrop` | Existing component | `web/src/ui/ThumbnailBackdrop.tsx`; rules arrive with the components tier |
| `toast`              | Existing component | `web/src/ui/Toast.tsx`; rules arrive with the components tier             |
| `tooltip`            | Existing component | `web/src/ui/Tooltip.tsx`; rules arrive with the components tier           |
