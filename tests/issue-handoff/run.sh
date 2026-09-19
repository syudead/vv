#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "$0")/../.." && pwd)
subject="$repo_root/scripts/issue-handoff/sdd-stage.sh"
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
assert_next upstream-revision-does-not-rewind taskstoissues "$repo"

repo=$(new_repo revised-ui-artifacts)
write_spec "$repo"
commit_all "$repo" spec
printf '# Plan\n' > "$repo/specs/901-example/plan.md"
commit_all "$repo" plan
printf '# UI design\n' > "$repo/specs/901-example/ui-design.md"
commit_all "$repo" design
printf '# Tasks\n\n- [ ] T001 Work\n' > "$repo/specs/901-example/tasks.md"
commit_all "$repo" tasks
printf '# Revised plan\n' > "$repo/specs/901-example/plan.md"
commit_all "$repo" revise-plan
printf '# Revised UI design\n' > "$repo/specs/901-example/ui-design.md"
commit_all "$repo" revise-design
assert_next revised-ui-artifacts-do-not-rewind taskstoissues "$repo" --ui

repo=$(new_repo dirty-artifacts)
write_spec "$repo"
commit_all "$repo" spec
printf '\nlocal edit\n' >> "$repo/specs/901-example/spec.md"
assert_failure dirty-artifacts "$repo"

repo=$(new_repo dirty-before-spec)
printf 'fixture\n' > "$repo/specs/901-example/.keep"
commit_all "$repo" init
printf 'local edit\n' > "$repo/specs/901-example/notes.md"
assert_failure dirty-before-spec "$repo"

repo=$(new_repo invalid-parent)
printf '# Spec\n\n**Parent Issue**: #42\n\n**Parent Issue**: #43\n' > "$repo/specs/901-example/spec.md"
commit_all "$repo" spec
assert_failure invalid-parent "$repo"

repo=$(new_repo invalid-parent-extra-text)
printf '# Spec\n\n**Parent Issue**: #42\n\n**Parent Issue**: #43 extra\n' > "$repo/specs/901-example/spec.md"
commit_all "$repo" spec
assert_failure invalid-parent-extra-text "$repo"

repo=$(new_repo invalid-parent-leading-space)
printf '# Spec\n\n **Parent Issue**: #42\n' > "$repo/specs/901-example/spec.md"
commit_all "$repo" spec
assert_failure invalid-parent-leading-space "$repo"

assert_exit missing-root 2 bash "$subject" --root "$tmp/does-not-exist" --feature specs/901-example
repo=$(new_repo non-normalized-feature)
write_spec "$repo"
commit_all "$repo" spec
assert_exit non-normalized-feature 2 bash "$subject" --root "$repo" --feature specs//901-example

wrapper="$repo_root/scripts/issue-handoff/run-speckit.sh"
repo=$(new_repo wrapper)
mkdir -p "$repo/.specify/scripts/bash"
cat > "$repo/.specify/scripts/bash/check-prerequisites.sh" <<'EOF'
#!/usr/bin/env bash
printf '{"feature_directory":"changed-by-speckit"}\n' > "$SPECIFY_INIT_DIR/.specify/feature.json"
printf 'feature=%s\n' "$SPECIFY_FEATURE_DIRECTORY"
sleep "${TEST_SLEEP:-0}"
[ "${1:-}" != "--fail" ]
EOF
cp "$repo/.specify/scripts/bash/check-prerequisites.sh" "$repo/.specify/scripts/bash/setup-plan.sh"
cp "$repo/.specify/scripts/bash/check-prerequisites.sh" "$repo/.specify/scripts/bash/setup-tasks.sh"
commit_all "$repo" wrapper-fixture

printf '{"feature_directory":"machine-local-selection"}\n' > "$repo/.specify/feature.json"
if output=$(cd "$repo" && bash "$wrapper" --feature specs/901-example prerequisites) &&
  printf '%s\n' "$output" | grep -qx 'feature=specs/901-example' &&
  grep -qx '{"feature_directory":"machine-local-selection"}' "$repo/.specify/feature.json"; then
  printf 'PASS wrapper-restores-existing-state\n'
  pass=$((pass + 1))
else
  printf 'FAIL wrapper-restores-existing-state\n' >&2
  fail=$((fail + 1))
fi

rm -f "$repo/.specify/feature.json"
if (cd "$repo" && bash "$wrapper" --feature specs/901-example plan >/dev/null) &&
  [ ! -e "$repo/.specify/feature.json" ]; then
  printf 'PASS wrapper-leaves-no-state\n'
  pass=$((pass + 1))
else
  printf 'FAIL wrapper-leaves-no-state\n' >&2
  fail=$((fail + 1))
fi

printf '{"feature_directory":"machine-local-selection"}\n' > "$repo/.specify/feature.json"
if ! (cd "$repo" && bash "$wrapper" --feature specs/901-example prerequisites --fail >/dev/null) &&
  grep -qx '{"feature_directory":"machine-local-selection"}' "$repo/.specify/feature.json"; then
  printf 'PASS wrapper-restores-state-after-failure\n'
  pass=$((pass + 1))
else
  printf 'FAIL wrapper-restores-state-after-failure\n' >&2
  fail=$((fail + 1))
fi

assert_exit wrapper-rejects-non-normalized-feature 2 bash -c \
  'cd "$1" && bash "$2" --feature specs//901-example prerequisites' _ "$repo" "$wrapper"

printf '{"feature_directory":"machine-local-selection"}\n' > "$repo/.specify/feature.json"
(cd "$repo" && bash "$wrapper" --feature specs/901-example prerequisites >/dev/null) &
pid_a=$!
sleep 0.2
(cd "$repo" && bash "$wrapper" --feature specs/901-example prerequisites >/dev/null) &
pid_b=$!
if wait "$pid_a" && wait "$pid_b" &&
  grep -qx '{"feature_directory":"machine-local-selection"}' "$repo/.specify/feature.json"; then
  printf 'PASS wrapper-serializes-state\n'
  pass=$((pass + 1))
else
  printf 'FAIL wrapper-serializes-state\n' >&2
  fail=$((fail + 1))
fi

printf '{"feature_directory":"machine-local-selection"}\n' > "$repo/.specify/feature.json"
TEST_SLEEP=2 bash -c 'cd "$1" && exec bash "$2" --feature specs/901-example prerequisites' \
  _ "$repo" "$wrapper" >/dev/null &
pid_a=$!
for _ in 1 2 3 4 5; do
  [ -f "$repo/.git/sdd-run-speckit.lock/owner" ] && [ -f "$repo/.specify/feature.json" ] && break
  sleep 0.2
done
bash -c 'cd "$1" && exec bash "$2" --feature specs/901-example prerequisites' \
  _ "$repo" "$wrapper" >/dev/null &
pid_b=$!
sleep 0.2
kill -TERM "$pid_b"
wait "$pid_b" >/dev/null 2>&1 || true
if [ -f "$repo/.specify/feature.json" ] && wait "$pid_a" &&
  grep -qx '{"feature_directory":"machine-local-selection"}' "$repo/.specify/feature.json"; then
  printf 'PASS wrapper-wait-interrupt-preserves-state\n'
  pass=$((pass + 1))
else
  printf 'FAIL wrapper-wait-interrupt-preserves-state\n' >&2
  fail=$((fail + 1))
fi

mkdir -p "$repo/.git/sdd-run-speckit.lock"
printf '999999\n' > "$repo/.git/sdd-run-speckit.lock/owner"
if (cd "$repo" && bash "$wrapper" --feature specs/901-example prerequisites >/dev/null) &&
  [ ! -e "$repo/.git/sdd-run-speckit.lock" ]; then
  printf 'PASS wrapper-recovers-abandoned-lock\n'
  pass=$((pass + 1))
else
  printf 'FAIL wrapper-recovers-abandoned-lock\n' >&2
  fail=$((fail + 1))
fi

contract_ok=true
for workflow in specify plan design tasks taskstoissues implement; do
  if [ ! -f "$repo_root/docs/agent-workflows/$workflow.md" ]; then
    printf 'missing canonical workflow: %s\n' "$workflow" >&2
    contract_ok=false
  fi
done
for adapter in speckit-specify speckit-plan speckit-tasks speckit-taskstoissues speckit-implement; do
  if ! grep -q 'docs/agent-workflows/' "$repo_root/.claude/skills/$adapter/SKILL.md"; then
    printf 'adapter does not reference canonical workflows: %s\n' "$adapter" >&2
    contract_ok=false
  fi
done
for adapter in speckit-plan speckit-tasks speckit-implement; do
  if ! grep -q 'scripts/issue-handoff/run-speckit.sh' "$repo_root/.claude/skills/$adapter/SKILL.md"; then
    printf 'adapter bypasses stateless Spec Kit wrapper: %s\n' "$adapter" >&2
    contract_ok=false
  fi
done
if ! bash -n "$repo_root/scripts/issue-handoff/run-speckit.sh"; then
  printf 'invalid stateless Spec Kit wrapper syntax\n' >&2
  contract_ok=false
fi
if [ -e "$repo_root/.claude/skills/sdd-next/SKILL.md" ]; then
  printf 'retired sdd-next controller still exists\n' >&2
  contract_ok=false
fi
for misplaced in \
  .specify/workflows/README.md \
  .specify/workflows/specify.md \
  .specify/workflows/plan.md \
  .specify/workflows/design.md \
  .specify/workflows/tasks.md \
  .specify/workflows/taskstoissues.md \
  .specify/workflows/implement.md \
  .specify/scripts/bash/sdd-stage.sh \
  .specify/tests/workflows/run.sh; do
  if [ -e "$repo_root/$misplaced" ]; then
    printf 'repository handoff file is misplaced under .specify: %s\n' "$misplaced" >&2
    contract_ok=false
  fi
done
if [ "$contract_ok" = true ]; then
  printf 'PASS repository-contract\n'
  pass=$((pass + 1))
else
  printf 'FAIL repository-contract\n' >&2
  fail=$((fail + 1))
fi

printf '%s passed, %s failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
