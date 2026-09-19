#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/../../.." && pwd)
subject="$repo_root/.specify/scripts/bash/sdd-stage.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

pass=0
fail=0

new_repo() {
  local name=$1
  local repo="$tmp/$name"
  mkdir -p "$repo/specs/901-example"
  git -C "$repo" init -q
  git -C "$repo" config user.name tests
  git -C "$repo" config user.email tests@example.invalid
  git -C "$repo" config core.autocrlf false
  printf '%s\n' "$repo"
}

commit_all() {
  local repo=$1 message=$2
  git -C "$repo" add .
  git -C "$repo" commit -qm "$message"
}

write_spec() {
  local repo=$1
  printf '# Spec\n\n**Parent Issue**: #42\n' > "$repo/specs/901-example/spec.md"
}

assert_next() {
  local name=$1 expected=$2 repo=$3
  shift 3
  local output
  if output=$(bash "$subject" --root "$repo" --feature specs/901-example "$@") &&
    printf '%s\n' "$output" | grep -qx "next=$expected"; then
    printf 'PASS %s\n' "$name"
    pass=$((pass + 1))
  else
    printf 'FAIL %s: expected next=%s\n%s\n' "$name" "$expected" "$output" >&2
    fail=$((fail + 1))
  fi
}

assert_failure() {
  local name=$1 repo=$2
  shift 2
  if bash "$subject" --root "$repo" --feature specs/901-example "$@" >/dev/null 2>&1; then
    printf 'FAIL %s: expected failure\n' "$name" >&2
    fail=$((fail + 1))
  else
    printf 'PASS %s\n' "$name"
    pass=$((pass + 1))
  fi
}

assert_exit() {
  local name=$1 expected=$2
  shift 2
  local actual=0
  "$@" >/dev/null 2>&1 || actual=$?
  if [ "$actual" -eq "$expected" ]; then
    printf 'PASS %s\n' "$name"
    pass=$((pass + 1))
  else
    printf 'FAIL %s: expected exit %s, got %s\n' "$name" "$expected" "$actual" >&2
    fail=$((fail + 1))
  fi
}

repo=$(new_repo missing-spec)
printf 'fixture\n' > "$repo/specs/901-example/.keep"
commit_all "$repo" init
assert_next missing-spec specify "$repo"

repo=$(new_repo before-plan)
write_spec "$repo"
commit_all "$repo" spec
assert_next before-plan plan "$repo"

repo=$(new_repo crlf-parent)
printf '# Spec\r\n\r\n**Parent Issue**: #42\r\n' > "$repo/specs/901-example/spec.md"
commit_all "$repo" spec
assert_next crlf-parent plan "$repo"

repo=$(new_repo before-tasks)
write_spec "$repo"
commit_all "$repo" spec
printf '# Plan\n' > "$repo/specs/901-example/plan.md"
commit_all "$repo" plan
assert_next before-tasks tasks "$repo"

repo=$(new_repo before-design)
write_spec "$repo"
commit_all "$repo" spec
printf '# Plan\n' > "$repo/specs/901-example/plan.md"
commit_all "$repo" plan
assert_next before-design design "$repo" --ui

repo=$(new_repo ui-before-tasks)
write_spec "$repo"
commit_all "$repo" spec
printf '# Plan\n' > "$repo/specs/901-example/plan.md"
commit_all "$repo" plan
printf '# UI design\n' > "$repo/specs/901-example/ui-design.md"
commit_all "$repo" design
assert_next ui-before-tasks tasks "$repo" --ui

repo=$(new_repo ready)
write_spec "$repo"
commit_all "$repo" spec
printf '# Plan\n' > "$repo/specs/901-example/plan.md"
commit_all "$repo" plan
printf '# Tasks\n\n- [ ] T001 Work\n' > "$repo/specs/901-example/tasks.md"
commit_all "$repo" tasks
git -C "$repo" switch -qc arbitrary-branch-name
assert_next ready taskstoissues "$repo"

printf '# Spec revised\n\n**Parent Issue**: #42\n' > "$repo/specs/901-example/spec.md"
commit_all "$repo" revise-spec
assert_next stale-plan plan "$repo"

repo=$(new_repo stale-design)
write_spec "$repo"
commit_all "$repo" spec
printf '# Plan\n' > "$repo/specs/901-example/plan.md"
commit_all "$repo" plan
printf '# UI design\n' > "$repo/specs/901-example/ui-design.md"
commit_all "$repo" design
printf '# Revised plan\n' > "$repo/specs/901-example/plan.md"
commit_all "$repo" revise-plan
assert_next stale-design design "$repo" --ui

repo=$(new_repo stale-tasks)
write_spec "$repo"
commit_all "$repo" spec
printf '# Plan\n' > "$repo/specs/901-example/plan.md"
commit_all "$repo" plan
printf '# UI design\n' > "$repo/specs/901-example/ui-design.md"
commit_all "$repo" design
printf '# Tasks\n\n- [ ] T001 Work\n' > "$repo/specs/901-example/tasks.md"
commit_all "$repo" tasks
printf '# Revised UI design\n' > "$repo/specs/901-example/ui-design.md"
commit_all "$repo" revise-design
assert_next stale-tasks tasks "$repo" --ui

repo=$(new_repo dirty-artifacts)
write_spec "$repo"
commit_all "$repo" spec
printf '\nlocal edit\n' >> "$repo/specs/901-example/spec.md"
assert_failure dirty-artifacts "$repo"

repo=$(new_repo invalid-parent)
printf '# Spec\n\n**Parent Issue**: #42\n\n**Parent Issue**: #43\n' > "$repo/specs/901-example/spec.md"
commit_all "$repo" spec
assert_failure invalid-parent "$repo"

assert_exit missing-root 2 bash "$subject" --root "$tmp/does-not-exist" --feature specs/901-example
repo=$(new_repo non-normalized-feature)
write_spec "$repo"
commit_all "$repo" spec
assert_exit non-normalized-feature 2 bash "$subject" --root "$repo" --feature specs//901-example

contract_ok=true
for workflow in specify plan design tasks taskstoissues implement; do
  if [ ! -f "$repo_root/.specify/workflows/$workflow.md" ]; then
    printf 'missing canonical workflow: %s\n' "$workflow" >&2
    contract_ok=false
  fi
done
for adapter in speckit-specify speckit-plan speckit-tasks speckit-taskstoissues speckit-implement; do
  if ! grep -q '\.specify/workflows/' "$repo_root/.claude/skills/$adapter/SKILL.md"; then
    printf 'adapter does not reference canonical workflows: %s\n' "$adapter" >&2
    contract_ok=false
  fi
done
if [ -e "$repo_root/.claude/skills/sdd-next/SKILL.md" ]; then
  printf 'retired sdd-next controller still exists\n' >&2
  contract_ok=false
fi
if [ -e "$repo_root/.specify/workflows/speckit/workflow.yml" ]; then
  printf 'retired full-cycle workflow still exists\n' >&2
  contract_ok=false
fi
if [ "$contract_ok" = true ]; then
  printf 'PASS repository-contract\n'
  pass=$((pass + 1))
else
  printf 'FAIL repository-contract\n' >&2
  fail=$((fail + 1))
fi

printf '%s passed, %s failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
