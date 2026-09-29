# UI design: {{Feature title}}

**Parent Issue**: #{{NNN}} | **Plan**: [plan.md](plan.md)

This document records only what this feature adds to or changes in the UI.
The visual rules come from
[library-ui.md](../../docs/design-docs/library-ui.md) and the tokens in
[`web/src/index.css`](../../web/src/index.css); name a token, never copy its
value.

## Screen boundary

| Screen | Route | Change |
| --- | --- | --- |
| {{Screen}} | `{{route}}` | {{What this feature adds or changes}} |

## Layout

{{A wireframe when the layout changes. Use a fenced block of box-drawing
characters or an image in `docs/screenshots/`.}}

```text
+--------------------------------------+
| {{region}}                             |
+--------------------------------------+
```

## Visual hierarchy

| Element | Rank | Treatment |
| --- | --- | --- |
| {{Element}} | {{Primary / secondary / tertiary}} | {{Tokens and type scale, e.g. `text-sm font-medium text-fg`}} |

## States

| State | Trigger | What the user sees | Available actions |
| --- | --- | --- | --- |
| Loading | {{trigger}} | {{view}} | {{actions}} |
| Empty | {{trigger}} | {{view}} | {{actions}} |
| Error | {{trigger}} | {{view}} | {{actions}} |

## Interactions

| Input | Target | Result |
| --- | --- | --- |
| {{Click / key / drag}} | {{Element}} | {{What happens}} |

## Responsive behaviour

| Width | Change |
| --- | --- |
| 360 px | {{Layout at the narrow width}} |
| 768 px | {{Layout at the medium width}} |
| 1280 px | {{Layout at the wide width}} |

## Copy

Proposed labels. After implementation, `web/src/i18n/en.ts` is the source of
truth.

| Key area | Label |
| --- | --- |
| {{Where}} | "{{Label}}" |

## Review criteria

Each criterion is judged by looking at the screen at the named width.

| # | Viewpoint | Criterion | Width |
| --- | --- | --- | --- |
| 1 | {{Hierarchy / density / spacing / typography / action priority}} | {{What must be true}} | {{px}} |
