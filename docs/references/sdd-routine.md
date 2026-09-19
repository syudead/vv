# Retired: Claude Routine `sdd-next`

The Claude Routine, `sdd` PR label trigger, and `/sdd-next` controller are no
longer part of this repository's SDD process. Do not recreate them from this
document.

Current operation starts by explicitly giving a parent Issue or native
sub-issue to a coding agent. The canonical procedure is
[`.specify/workflows/README.md`](../../.specify/workflows/README.md), and the
design rationale is
[`docs/design-docs/sdd-loop-harness.md`](../design-docs/sdd-loop-harness.md).

## External cleanup

Repository changes cannot delete a Routine configured in claude.ai. A
maintainer must disable and then delete the old `sdd-next (syudead/vv)` Routine
after the Issue handoff flow has been verified on GitHub. Remove the repository
`sdd` label only after no open legacy PR still relies on it.

The old trigger prompt, schedule, branch filters, retry limits, and usage gate
are intentionally not preserved here. Git history contains the retired
configuration if it is needed for an audit.
