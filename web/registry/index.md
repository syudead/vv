# vv registry index

The vv registry lists the components, tokens, rules and page patterns that every
screen in `web/src` is built from. Read an item from `web/` with
`npx shadcn view ./registry/r/<item>.json`; the design decisions are in
[design-system.md](../../docs/design-docs/design-system.md).

| Item                 | Tier                  | When to use                                                              |
| -------------------- | --------------------- | ------------------------------------------------------------------------ |
| `vv`                 | Index                 | This list                                                                |
| `vv-theme`           | Foundations           | `web/src/ui/tokens.css`: every token; read before styling anything       |
| `vv-rules`           | Rules                 | `web/registry/rules/`: which token, step and component to use where      |
| `alert`              | Overlays and feedback | A message inside a section: error, warning, done, guidance               |
| `alert-dialog`       | Overlays and feedback | Confirm an action that cannot be undone                                  |
| `badge`              | Overlays and feedback | Tag chips, counts, status labels                                         |
| `brand-home-link`    | vv component          | The logo link in the top bar                                             |
| `breadcrumb`         | Overlays and feedback | The path to the current folder                                           |
| `button`             | Existing component    | `web/src/ui/Button.tsx`; rules arrive with the components tier           |
| `checkbox`           | Existing component    | `web/src/ui/Checkbox.tsx`; rules arrive with the components tier         |
| `combobox`           | Existing component    | `web/src/ui/Combobox.tsx`; rules arrive with the components tier         |
| `dialog`             | Overlays and feedback | A modal form or long choice                                              |
| `dropdown-menu`      | Overlays and feedback | Actions or one choice from a few, from a button                          |
| `empty`              | Overlays and feedback | The empty state of a list or section                                     |
| `favorite-toggle`    | vv component          | The favorite heart: mark and toggle                                      |
| `filter-chip`        | Existing component    | `web/src/ui/FilterChip.tsx`; rules arrive with the components tier       |
| `icon-button`        | Existing component    | `web/src/ui/IconButton.tsx`; rules arrive with the components tier       |
| `kbd`                | Overlays and feedback | A key or search operator in text                                         |
| `popover`            | Overlays and feedback | Options or a short form anchored to a control                            |
| `progress`           | Overlays and feedback | Progress of a known amount of work                                       |
| `scrub-preview`      | vv component          | The card scrub inside a thumbnail                                        |
| `segmented-control`  | Existing component    | `web/src/ui/SegmentedControl.tsx`; rules arrive with the components tier |
| `separator`          | Overlays and feedback | A hairline between groups                                                |
| `sheet`              | Overlays and feedback | Edge panel; only the sidebar's narrow-width drawer                       |
| `sidebar`            | Overlays and feedback | The app's main navigation                                                |
| `skeleton`           | Overlays and feedback | Shapes of the final layout while loading                                 |
| `sonner`             | Overlays and feedback | Short passive notices (`toast()`)                                        |
| `spinner`            | Overlays and feedback | A short wait with no measure                                             |
| `tabs`               | Overlays and feedback | Switch views of the same content in place                                |
| `tentative-mark`     | vv component          | The mark after a tentative tag                                           |
| `thumbnail-backdrop` | vv component          | Blurred fill behind a portrait thumbnail                                 |
| `tooltip`            | Overlays and feedback | Name an icon-only control, a short hint                                  |
| `video-thumbnail`    | vv component          | The thumbnail frame of cards and rows                                    |
