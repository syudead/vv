# Components rules

Every control on a new or migrated screen is a component from
`web/src/ui/shadcn`, read through its registry item. `task check` fails on a raw `<button>`, `<input>`, `<select>`
or `<textarea>` outside `web/src/ui`. Each component is the shadcn/ui
component on the radix base, dressed only through the tokens of
[foundations.md](foundations.md); its structure and variants are upstream's.
The reasons are in
[design-system.md, Components](../../../docs/design-docs/design-system.md#components).

## Shared rules

- **Focus**: every component shows the same ring, the `:focus-visible`
  outline in `web/src/index.css`. Never add `outline-none` or a focus ring of
  your own.
- **States**: hover, pressed, selected and disabled come from the component.
  Do not restyle them per screen; disabled is `opacity-50` everywhere.
- **Size by density**: the library uses `sm` (`h-8`) controls and `icon-sm`
  icon buttons; the video page uses `default` (`h-9`) and `lg` (`h-10`) for its
  main action. Icon-only buttons in one area share one size.
- **Names**: an icon-only control has an `aria-label` from the i18n catalog and
  a tooltip with the same text.
- **Text**: components take their text from the caller through `t`; they hold
  none of their own.

## Button

Item `button`. An action that happens when pressed.

| Variant       | Use it for                                                                           |
| ------------- | ------------------------------------------------------------------------------------ |
| `default`     | The one main action of an area (a dialog, a form, an empty state); at most one       |
| `outline`     | Secondary actions next to a `default` one, and toolbar triggers (filter, sort, view) |
| `secondary`   | A quieter filled action where an outline would be too strong                         |
| `ghost`       | Actions in a dense row: the selection bar, menus' triggers, clearing a field         |
| `destructive` | The action that deletes or rejects, named by its verb, in a confirmation             |
| `link`        | An action that reads as a link inside text                                           |

Sizes are `sm`, `default` and `lg`, and `icon-sm` and `icon` for icon-only
buttons. Combine an icon-only button with `Tooltip`; put a link that looks like
a button through `asChild` (`<Button asChild><Link /></Button>`), so it stays a
link.

Do not use a Button for navigation that is a plain link in text, to show a
state that stays on (use `Toggle`), or to pick one of several options (use
`ToggleGroup` or `RadioGroup`). Do not put two `default` buttons in one area.

## Input, Textarea, Label and Field

Items `input`, `textarea`, `label`, `field`. Text the viewer types.

- `Input` takes one line (a name, a search, a token); `Textarea` takes
  several lines and grows with its content.
- Every field has a visible `Label` bound with `htmlFor`, or, in a toolbar
  where the placeholder names it, an `aria-label`.
- Wrap a form field in `Field` with `FieldLabel`, the control,
  `FieldDescription` for help and `FieldError` for the reason it is wrong; set
  `aria-invalid` on the control at the same time. Group related fields in
  `FieldGroup`, or in `FieldSet` with `FieldLegend` when they answer one
  question.
- `Field orientation="horizontal"` puts a `Checkbox` or `Switch` before its
  label; a settings row (label and description on the left, control on the
  right) also uses it, with `FieldContent` around the text.

Do not use `Input` for a choice among known values (use `Select`,
`RadioGroup` or `Combobox`), or a `Textarea` for a single line.

## Select and RadioGroup

Items `select`, `radio-group`. Pick one value from a fixed list.

- `RadioGroup` when the options are few (up to about six) and seeing all of
  them helps the choice, as for the sort order in the narrow toolbar. Each
  `RadioGroupItem` has a `Label`.
- `Select` when the list is longer, or space is short and the current value is
  what matters, as in a settings row.

Do not use either for a choice that acts at once in a toolbar (use
`ToggleGroup`), for many values the viewer would search (use `Combobox`), or
for an on and off setting (use `Switch` or `Checkbox`).

## Checkbox and Switch

Items `checkbox`, `switch`. Turn one thing on or off.

- `Checkbox` is a choice inside a form, a filter popover or a selection, and
  takes effect with the rest of it. It has an `indeterminate` state for a
  group partly selected. A card's selection mark is a `Checkbox` with an
  `aria-label` that names the item.
- `Switch` is a setting that takes effect at once, such as a video's
  visibility.

Pair either with a `Label` (`Field orientation="horizontal"`). Do not use a
`Switch` inside a form that is saved with a button, or a `Checkbox` to run an
action.

## Toggle and ToggleGroup

Items `toggle`, `toggle-group`. Buttons that stay pressed.

- `Toggle` is one state that stays on: an applied filter chip, which the
  viewer unpresses to remove. Its name is the thing it applies (the tag name),
  and its `title` says that pressing removes it.
- `ToggleGroup type="single"` is an exclusive choice that acts at once: view
  mode, sort direction. Its items are radios to assistive technology. Use
  `variant="outline"` and the area's size; icon-only items get an
  `aria-label` and a `Tooltip`.
- `ToggleGroup type="multiple"` is a row of independent toggles, such as text
  formatting.

The pressed state is `primary-soft` with `primary` text, the same as a
selected row. Do not use a Toggle for a one-off action (use `Button`) or for a
choice inside a form that is saved later (use `RadioGroup` or `Checkbox`).

## Slider

Item `slider`. Pick a value on a range by dragging or with the arrow keys.

Use it for a value whose exact number does not matter, such as the card size
(four steps). Give it an `aria-label`; the label goes on the thumb, which takes
the focus. Do not use it when the viewer must hit an exact value (use an
`Input` or a `Select`).

## Combobox and Command

Items `combobox`, `command`. Pick one value from many by typing.

- `Combobox` is a button showing the current value; it opens a `Command` in a
  popover with a search field and the matching options. Use it for a long
  list (folders, tags) where typing is faster than scrolling.
- `Command` is the filtered list on its own. Put it inside an existing
  popover when the popover itself is the picker, such as a selection bar's
  "Add tag" and "Remove tag"; pass `shouldFilter={false}` when the caller
  orders and filters the options. Give `Command` and `CommandList` a `label`
  from the catalog; cmdk's own default names are English text outside it.

Enter takes the selected row. When typing can create a value, put the create
row first, so Enter creates unless the viewer moved to another row. Do not use
a Combobox for a handful of options (use `Select` or `RadioGroup`).
