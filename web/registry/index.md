# vv registry index

The vv registry lists the components, tokens, rules and page patterns that every
screen in `web/src` is built from. Read an item from `web/` with
`npx shadcn view ./registry/r/<item>.json`; the design decisions are in
[design-system.md](../../docs/design-docs/design-system.md).

| Item                 | Tier               | When to use                                                               |
| -------------------- | ------------------ | ------------------------------------------------------------------------- |
| `vv`                 | Index              | This list                                                                 |
| `vv-theme`           | Foundations        | `web/src/ui/tokens.css`: every token; read before styling anything        |
| `vv-rules`           | Rules              | `web/registry/rules/`: which token, step and component to use where       |
| `brand-home-link`    | Existing component | `web/src/ui/BrandHomeLink.tsx`; rules arrive with the components tier     |
| `button`             | Component          | Actions; one `default` per area (rules: components.md)                    |
| `checkbox`           | Component          | On or off inside a form, a filter or a selection                          |
| `chip`               | Existing component | `web/src/ui/Chip.tsx`; rules arrive with the components tier              |
| `combobox`           | Component          | Pick one of many by typing; a `command` in a popover                      |
| `command`            | Component          | A list narrowed by typing; the inside of a combobox                       |
| `field`              | Component          | Label, control, description and error of one form field                   |
| `input`              | Component          | One line of text                                                          |
| `label`              | Component          | The visible name of a control                                             |
| `menu`               | Existing component | `web/src/ui/Menu.tsx`; rules arrive with the components tier              |
| `modal-frame`        | Existing component | `web/src/ui/ModalFrame.tsx`; rules arrive with the components tier        |
| `popover`            | Existing component | `web/src/ui/Popover.tsx`; rules arrive with the components tier           |
| `radio-group`        | Component          | Pick one of a few visible options                                         |
| `scrub-preview`      | Existing component | `web/src/ui/ScrubPreview.tsx`; rules arrive with the components tier      |
| `select`             | Component          | Pick one value from a fixed list in a dropdown                            |
| `skeleton`           | Existing component | `web/src/ui/Skeleton.tsx`; rules arrive with the components tier          |
| `slider`             | Component          | A value on a range, such as the card size                                 |
| `switch`             | Component          | A setting that takes effect at once                                       |
| `tabs`               | Existing component | `web/src/ui/Tabs.tsx`; rules arrive with the components tier              |
| `tentative-mark`     | Existing component | `web/src/ui/TentativeMark.tsx`; rules arrive with the components tier     |
| `textarea`           | Component          | Several lines of text                                                     |
| `thumbnail-backdrop` | Existing component | `web/src/ui/ThumbnailBackdrop.tsx`; rules arrive with the components tier |
| `toast`              | Existing component | `web/src/ui/Toast.tsx`; rules arrive with the components tier             |
| `toggle-group`       | Component          | Exclusive or independent toggles: view mode, direction                    |
| `toggle`             | Component          | A button that stays pressed, such as an applied filter                    |
| `tooltip`            | Existing component | `web/src/ui/Tooltip.tsx`; rules arrive with the components tier           |
