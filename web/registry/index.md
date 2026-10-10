# vv registry index

The vv registry lists the components, tokens, rules and page patterns that every
screen in `web/src` is built from. Read an item from `web/` with
`npx shadcn view ./registry/r/<item>.json`; the design decisions are in
[design-system.md](../../docs/design-docs/design-system.md).

| Item                       | Tier          | When to use                                                                  |
| -------------------------- | ------------- | ---------------------------------------------------------------------------- |
| `vv`                       | Index         | This list                                                                    |
| `vv-theme`                 | Foundations   | `web/src/ui/tokens.css`: every token; read before styling anything           |
| `vv-rules`                 | Rules         | `web/registry/rules/`: which token, step, component and pattern to use where |
| `admin-table-page`         | Page skeleton | Records in rows with bulk actions                                            |
| `admin-table-page-example` | Block         | Copy to start an admin table page                                            |
| `alert`                    | Component     | A message inside a section: error, warning, done, guidance                   |
| `alert-dialog`             | Component     | Confirm an action that cannot be undone                                      |
| `badge`                    | Component     | Tag chips, counts, status labels                                             |
| `brand-home-link`          | vv component  | The logo link in the top bar                                                 |
| `breadcrumb`               | Component     | The path to the current folder                                               |
| `button`                   | Component     | Actions; one `default` per area (rules: components.md)                       |
| `card-grid`                | Section       | Cards whose edges line up with the toolbar                                   |
| `centered-form`            | Page skeleton | A screen that is only one form: sign-in, setup                               |
| `centered-form-example`    | Block         | Copy to start a centered form                                                |
| `checkbox`                 | Component     | On or off inside a form, a filter or a selection                             |
| `combobox`                 | Component     | Pick one of many by typing; a `command` in a popover                         |
| `command`                  | Component     | A list narrowed by typing; the inside of a combobox                          |
| `confirm-dialog`           | Page skeleton | Confirm an action that cannot be undone                                      |
| `confirm-dialog-example`   | Block         | Copy to start a confirm dialog                                               |
| `data-table`               | Section       | A table on a card                                                            |
| `density`                  | Pattern       | The density a skeleton gives its sections; patterns only                     |
| `detail-page`              | Page skeleton | One item to view, with its facts beside it                                   |
| `detail-page-example`      | Block         | Copy to start a detail page                                                  |
| `dialog`                   | Component     | A modal form or long choice                                                  |
| `dropdown-menu`            | Component     | Actions or one choice from a few, from a button                              |
| `empty`                    | Component     | The empty state of a list or section                                         |
| `empty-state`              | State         | Nothing to show, and the action that fills it                                |
| `error-state`              | State         | The body failed to load, with Retry                                          |
| `fact-list`                | Section       | Term and value pairs                                                         |
| `favorite-toggle`          | vv component  | The favorite heart: mark and toggle                                          |
| `field`                    | Component     | Label, control, description and error of one form field                      |
| `form-dialog`              | Page skeleton | A few values asked without leaving the screen                                |
| `form-dialog-example`      | Block         | Copy to start a form dialog                                                  |
| `grouped-list`             | Section       | Rows under headings that stick, such as days                                 |
| `grouped-list-example`     | Block         | Copy to start a list of rows grouped under headings                          |
| `form-row`                 | Section       | One setting: label, description, control                                     |
| `input`                    | Component     | One line of text                                                             |
| `jump-list`                | Section       | Jump to a day or month from a side column or a strip of chips                |
| `kbd`                      | Component     | A key or search operator in text                                             |
| `label`                    | Component     | The visible name of a control                                                |
| `list-page`                | Page skeleton | Items to browse, filter and select (rules: patterns.md)                      |
| `list-page-example`        | Block         | Copy to start a list page                                                    |
| `list-states-example`      | Block         | The list body in every state                                                 |
| `load-more-row`            | State         | The next page loading, or failed with Retry                                  |
| `loading-state`            | State         | Skeleton shapes of the final layout: grid, table or timeline                 |
| `page-header`              | Section       | Page title, count and the main action                                        |
| `page-section`             | Section       | A titled card of rows                                                        |
| `popover`                  | Component     | Options or a short form anchored to a control                                |
| `progress`                 | Component     | Progress of a known amount of work                                           |
| `radio-group`              | Component     | Pick one of a few visible options                                            |
| `scrub-preview`            | vv component  | The card scrub inside a thumbnail                                            |
| `select`                   | Component     | Pick one value from a fixed list in a dropdown                               |
| `selection-bar`            | Section       | Count and bulk actions while items are selected                              |
| `separator`                | Component     | A hairline between groups                                                    |
| `settings-page`            | Page skeleton | Settings, one value per row                                                  |
| `settings-page-example`    | Block         | Copy to start a settings page                                                |
| `sheet`                    | Component     | Edge panel; only the sidebar's narrow-width drawer                           |
| `sidebar`                  | Component     | The app's main navigation                                                    |
| `skeleton`                 | Component     | Shapes of the final layout while loading                                     |
| `slider`                   | Component     | A value on a range, such as the card size                                    |
| `sonner`                   | Component     | Short passive notices (`toast()`)                                            |
| `spinner`                  | Component     | A short wait with no measure                                                 |
| `switch`                   | Component     | A setting that takes effect at once                                          |
| `table`                    | Component     | Rows of records; only inside a `data-table`                                  |
| `tabs`                     | Component     | Switch views of the same content in place                                    |
| `tag-command`              | vv component  | Type a tag name to pick or create a tag                                      |
| `tentative-mark`           | vv component  | The mark after a tentative tag                                               |
| `textarea`                 | Component     | Several lines of text                                                        |
| `thumbnail-backdrop`       | vv component  | Blurred fill behind a portrait thumbnail                                     |
| `timeline`                 | Section       | Rows on a vertical rule grouped by day, such as a history                    |
| `timeline-example`         | Block         | Copy to start a day timeline of rows                                         |
| `toggle`                   | Component     | A button that stays pressed, such as an applied filter                       |
| `toggle-group`             | Component     | Exclusive or independent toggles: view mode, direction                       |
| `toolbar`                  | Section       | Search, filters, view controls of a list                                     |
| `tooltip`                  | Component     | Name an icon-only control, a short hint                                      |
| `video-thumbnail`          | vv component  | The thumbnail frame of cards and rows                                        |
