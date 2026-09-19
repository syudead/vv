---
name: "sdd-design"
description: "Create the UI and interaction design artifact for an Issue-driven SDD feature."
argument-hint: "Parent GitHub Issue URL or number"
compatibility: "Requires this repository's .specify/workflows directory"
user-invocable: true
disable-model-invocation: false
---

# UI design adapter

Read and execute `.specify/workflows/README.md` and
`.specify/workflows/design.md`. This adapter adds no branch naming, state,
trigger, or retry protocol. Produce only the Design-stage artifact and PR,
then stop.

