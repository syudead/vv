# Contract: [interface name]

<!--
  Type: contract. Rules: docs/design-docs/writing-quality.md (W-rules) and
  docs/design-docs/plan-quality.md (P-2: only when this feature adds or changes
  an interface).

  When a machine-readable schema is the source of truth (api/openapi.yaml,
  api/external-v1.yaml), name it and describe only what this feature adds or
  changes. One `##` section per endpoint, command, or message.
-->

Source of truth: [link to the schema file and the operation ids].

## `[METHOD /path]`

[What it does, in one sentence.]

<!-- DIAGRAM (Mermaid sequenceDiagram) of the caller, this endpoint and what
     it calls, when the exchange has more than one round trip or branches by
     outcome (W-7). -->

<!-- TABLE: parameters or request fields. -->

| Field | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| | | | | |

**Response**: [Status and body. Link to the schema rather than copying it.]

<!-- TABLE: one row per error the client must handle. -->

| Status | `code` | When |
| --- | --- | --- |
| | | |

## Client use

[How the caller uses the interface, when that is not obvious from the
endpoint. Delete otherwise.]
