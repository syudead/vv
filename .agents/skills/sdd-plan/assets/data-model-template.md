# Data model: [FEATURE]

<!--
  Type: data model. Rules: docs/design-docs/writing-quality.md (W-rules) and
  docs/design-docs/plan-quality.md (P-2: only when this feature adds or changes
  an entity).

  Write only the entities this feature adds and the fields it changes. Say that
  the rest of the model is unchanged rather than restating it.
-->

The rest of the model is unchanged: [link to the canonical schema or migrations].

## Migration

[The migration this feature adds: file name, and what it creates or alters.]

## `[table_or_entity]`

<!-- TABLE: one row per field. -->

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| | | | |

**Relationships**: [Foreign keys and what happens on delete.]

## Rules

<!-- TABLE: one row per invariant or validation rule, with where it is enforced. -->

| Rule | Enforced in |
| --- | --- |
| | |

## State transitions

<!-- DIAGRAM (Mermaid stateDiagram) when an entity has states. Delete otherwise. -->

## What does not change

[Entities or fields a reader might expect to change, and why they do not.]
