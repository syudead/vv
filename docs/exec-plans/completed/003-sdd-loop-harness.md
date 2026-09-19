# Completed: Claude Routine SDD loop

- Status: Retired on 2026-09-19
- Original implementation completed: 2026-09-13
- Replacement: [Issue handoff execution plan](../active/011-agent-agnostic-issue-handoff.md)

The original work implemented `/sdd-next`, Git-derived guards, a Claude
Routine trigger, automatic stage selection, and automatic merge behavior. It
was operationally complete but coupled stage continuation to Claude-specific
session and trigger behavior.

The implementation history remains in Git and in completed plans 004, 006,
009, and 010. The live controller, tests, and usage hook were removed when the
repository adopted the agent-neutral Issue handoff design. The current
specification and design are:

- [`specs/003-sdd-loop-harness/spec.md`](../../../specs/003-sdd-loop-harness/spec.md)
- [`docs/design-docs/sdd-loop-harness.md`](../../design-docs/sdd-loop-harness.md)
- [`.specify/workflows/README.md`](../../../.specify/workflows/README.md)

The external Claude Routine and legacy `sdd` label require maintainer cleanup
after live GitHub verification because repository changes cannot delete them.

