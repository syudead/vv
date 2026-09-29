# Contract: {{Interface name}}

**Plan**: [plan.md](plan.md) | **Schema**: [api/openapi.yaml](../../../api/openapi.yaml)

{{One sentence: who calls this and what for.}}

## `{{METHOD}} {{path}}`

| | |
| --- | --- |
| **Auth** | {{owner / guest / token scope}} |
| **Idempotent** | {{yes / no}} |

**Request**:

| Parameter | In | Type | Required | Meaning |
| --- | --- | --- | --- | --- |
| `{{name}}` | {{path / query / body}} | `{{type}}` | {{yes / no}} | {{Meaning and allowed values.}} |

**Responses**:

| Status | Body | When |
| --- | --- | --- |
| `200` | `{{Schema}}` | {{Condition}} |
| `400` | `Error` with `code: {{code}}` | {{Condition}} |

**Example**:

```http
{{METHOD}} {{path}}
```

```json
{}
```
