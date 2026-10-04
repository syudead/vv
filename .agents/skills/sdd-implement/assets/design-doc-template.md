# [Subsystem or decision]

<!--
  Type: design document. Rules: docs/design-docs/writing-quality.md (W-rules)
  and the policy in docs/design-docs/index.md: write what is true now and why,
  not restrictions. Add the document to docs/design-docs/index.md.

  One `##` section per decision. Delete the parts a section does not need.
  Record the rule and its reason, not how the code implements it (W-10).
  Budget per section: W-11.
-->

[One sentence: what this subsystem does and where its code lives.]

[One sentence: what the diagram shows.]

```mermaid
flowchart LR
  %% Required: the parts this document covers and how a request or job
  %% moves between them (W-7).
```

## [Decision]

[The rule, in one sentence. Link the file that implements it.]

[Why, in at most three sentences: the constraint that forced the choice.]

<!-- DIAGRAM (Mermaid) when the section is a flow, a sequence of calls, a
     state change or a branching rule (W-7): flowchart, sequenceDiagram or
     stateDiagram-v2. Most sections have one. -->

<!-- TABLE for cases, modes, or settings and the behaviour a user or caller
     sees in each. Not a list of functions. -->

| Rejected | Why |
| --- | --- |
| [Alternative] | [One sentence] |
