# vv registry index

The vv registry lists the components, tokens, rules and page patterns that every
screen in `web/src` is built from. Read an item from `web/` with
`npx shadcn view ./registry/r/<item>.json`; the design decisions are in
[design-system.md](../../docs/design-docs/design-system.md).

| Item                 | Tier         | When to use                                                         |
| -------------------- | ------------ | ------------------------------------------------------------------- |
| `vv`                 | Index        | This list                                                           |
| `vv-theme`           | Foundations  | `web/src/ui/tokens.css`: every token; read before styling anything  |
| `vv-rules`           | Rules        | `web/registry/rules/`: which token, step and component to use where |
| `alert`              | Component    | A message inside a section: error, warning, done, guidance          |
| `alert-dialog`       | Component    | Confirm an action that cannot be undone                             |
| `badge`              | Component    | Tag chips, counts, status labels                                    |
| `brand-home-link`    | vv component | The logo link in the top bar                                        |
| `breadcrumb`         | Component    | The path to the current folder                                      |
| `button`             | Component    | Actions; one `default` per area (rules: components.md)              |
| `checkbox`           | Component    | On or off inside a form, a filter or a selection                    |
| `combobox`           | Component    | Pick one of many by typing; a `command` in a popover                |
| `command`            | Component    | A list narrowed by typing; the inside of a combobox                 |
| `dialog`             | Component    | A modal form or long choice                                         |
| `dropdown-menu`      | Component    | Actions or one choice from a few, from a button                     |
| `empty`              | Component    | The empty state of a list or section                                |
| `favorite-toggle`    | vv component | The favorite heart: mark and toggle                                 |
| `field`              | Component    | Label, control, description and error of one form field             |
| `input`              | Component    | One line of text                                                    |
| `kbd`                | Component    | A key or search operator in text                                    |
| `label`              | Component    | The visible name of a control                                       |
| `popover`            | Component    | Options or a short form anchored to a control                       |
| `progress`           | Component    | Progress of a known amount of work                                  |
| `radio-group`        | Component    | Pick one of a few visible options                                   |
| `scrub-preview`      | vv component | The card scrub inside a thumbnail                                   |
| `select`             | Component    | Pick one value from a fixed list in a dropdown                      |
| `separator`          | Component    | A hairline between groups                                           |
| `sheet`              | Component    | Edge panel; only the sidebar's narrow-width drawer                  |
| `sidebar`            | Component    | The app's main navigation                                           |
| `skeleton`           | Component    | Shapes of the final layout while loading                            |
| `slider`             | Component    | A value on a range, such as the card size                           |
| `sonner`             | Component    | Short passive notices (`toast()`)                                   |
| `spinner`            | Component    | A short wait with no measure                                        |
| `switch`             | Component    | A setting that takes effect at once                                 |
| `tabs`               | Component    | Switch views of the same content in place                           |
| `tentative-mark`     | vv component | The mark after a tentative tag                                      |
| `textarea`           | Component    | Several lines of text                                               |
| `thumbnail-backdrop` | vv component | Blurred fill behind a portrait thumbnail                            |
| `toggle`             | Component    | A button that stays pressed, such as an applied filter              |
| `toggle-group`       | Component    | Exclusive or independent toggles: view mode, direction              |
| `tooltip`            | Component    | Name an icon-only control, a short hint                             |
| `video-thumbnail`    | vv component | The thumbnail frame of cards and rows                               |
