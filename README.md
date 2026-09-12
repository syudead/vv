# vv

An agent-friendly repository scaffold based on OpenAI's harness engineering
example. The repository keeps its operating instructions short at the root and
stores detailed, durable context under `docs/`.

## Repository structure

```text
.
├── AGENTS.md
├── ARCHITECTURE.md
└── docs/
    ├── design-docs/
    │   ├── core-beliefs.md
    │   ├── index.md
    │   └── tech-stack-selection.md
    ├── exec-plans/
    │   ├── active/
    │   │   └── README.md
    │   ├── completed/
    │   │   └── README.md
    │   └── tech-debt.md
    ├── generated/
    │   └── README.md
    ├── product-specs/
    │   └── index.md
    └── references/
        └── README.md
```

Directory-level README files are included so that intentionally empty
directories remain visible in Git and explain what belongs in each location.
