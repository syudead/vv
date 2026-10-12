# Improvement scout playbook

Choose a narrow lens for each run and verify candidates against actual code paths. Record useful lessons here so subsequent runs can explore new ground instead of repeating the same proposals.

| Lens | Look for | Last run | New proposals | Note for next run |
| --- | --- | --- | ---: | --- |
| Slow developer feedback | Repeated work in build, test, generation, or local startup | — | 0 | — |
| Error recovery | Failures that leave jobs, scans, or artifacts stuck until manual intervention | — | 0 | — |
| Data and artifact lifecycle | Stale files, lost state, redundant writes, or expensive cleanup | — | 0 | — |
| API and UI consistency | Mismatches between API behavior, generated contract, and visible UI | — | 0 | — |
| Accessibility and interaction | Keyboard, touch, focus, and feedback gaps in real flows | — | 0 | — |
| Test reliability | Flaky timing, hidden external dependencies, or missing high-value coverage | — | 0 | — |

## Decisions learned from triage

When a proposal is rejected, move its file from `proposed/` to `archive/` and append a short `## Rejection reason` to that file. This is the reference for duplicate checks; preserve the original evidence and decision.
