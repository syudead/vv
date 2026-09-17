# Local development review fixes

- Require executable Git and reject failed or empty version probes in doctor.
- Read tool versions from one JSON file in Make and PowerShell.
- Run PowerShell regression tests on Windows and Linux CI.
- Validate locally, reply to PR #47 reviews, and inspect CI results.

## Validation

- task doctor and task check passed locally.
- Nine PowerShell regression cases passed, including missing Git, failed jq,
  empty version output, and a healthy environment.
- Go tests/lint, Web build/type checks and 156 tests, 25 SDD tests and generated
  API consistency checks passed.
- CI now runs the PowerShell tests on both Windows and Linux. Remote results
  are tracked on PR #47 after push.
- UI unchanged.
