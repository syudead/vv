# Local check coverage

- Pin jq and require it for local checks.
- Exercise scanner file-read failures independently of OS permissions.
- Retain the real chmod test as supplementary coverage.
- Run all checks, including the ten SDD guard cases, and commit the results.

## Results

- jq 1.8.1 installed through mise; doctor reports it as required.
- Missing or failing jq now fails checks; a failure-injection test covers this.
- Scanner uses its usual ContentKey function by default. The regression test
  replaces that function on one scanner instance to return a permission error
  for the middle file, while processing the other two real files normally.
- TestScanContinuesAfterFileFailure passed on Windows without skipping:
  total=3, failed=1, added=2, both surrounding files indexed.
- The existing chmod integration test remains supplementary and skips where
  chmod cannot prohibit reading (Windows or privileged users).
- task setup, task doctor and task check passed. SDD: 25 passed, 0 failed,
  no skipped guard cases. Web: 156 tests passed. Go tests, lint, formatting,
  Web build and generated API checks passed.
- UI unchanged. Docker remains outside this verification.
