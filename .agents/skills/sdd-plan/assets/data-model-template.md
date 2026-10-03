# Data model: [FEATURE]

<!--
  Type: data model. Rules: docs/design-docs/writing-quality.md (W-rules) and
  docs/design-docs/plan-quality.md (P-2: only when this feature adds or changes
  an entity).

  Write only the entities this feature adds and the fields it changes. Say that
  the rest of the model is unchanged rather than restating it.
-->

The rest of the model is unchanged: [link to the canonical schema or migrations].

## 1. Migration

[The migration this feature adds: file name, and what it creates or alters.]

## 2. `[table_or_entity]`

<!-- TABLE: one row per field. -->

| Field | Type | Null | Meaning |
| --- | --- | --- | --- |
| | | | |

**Relationships**: [Foreign keys and what happens on delete.]

## 3. Rules

<!-- TABLE: one row per invariant or validation rule, with where it is enforced. -->

| Rule | Enforced in |
| --- | --- |
| | |

## 4. State transitions

<!-- DIAGRAM (Mermaid stateDiagram) when an entity has states. Delete otherwise. -->

## 5. What does not change

[Entities or fields a reader might expect to change, and why they do not.]
