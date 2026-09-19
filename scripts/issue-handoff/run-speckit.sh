#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'usage: run-speckit.sh --feature specs/NNN-name <plan|tasks|prerequisites> [args...]\n' >&2
  exit 2
}

feature=""
if [ "${1:-}" = "--feature" ] && [ "$#" -ge 3 ]; then
  feature=$2
  shift 2
fi
[ -n "$feature" ] || usage

case "$feature" in
  specs/*) ;;
  *) printf 'run-speckit: feature must be under specs/: %s\n' "$feature" >&2; exit 2 ;;
esac
case "/$feature/" in
  */../*|*/./*|*//*)
    printf 'run-speckit: feature path must be normalized: %s\n' "$feature" >&2
    exit 2
    ;;
esac

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  printf 'run-speckit: not inside a git repository\n' >&2
  exit 2
}
state="$repo_root/.specify/feature.json"
lock=$(git -C "$repo_root" rev-parse --git-path sdd-run-speckit.lock)
backup=$(mktemp)
had_state=false
lock_acquired=false

acquire_lock() {
  while ! mkdir "$lock" 2>/dev/null; do
    sleep 1
  done
  lock_acquired=true
}

record_input() {
  local upstream=$1 downstream=$2 upstream_commit tmp_marker
  upstream_commit=$(git -C "$repo_root" log -1 --format=%H -- "$upstream")
  [ -n "$upstream_commit" ] || return 0
  [ -f "$repo_root/$downstream" ] || return 0

  tmp_marker=$(mktemp)
  printf '<!-- SDD input: %s @ %s -->\n' "$upstream" "$upstream_commit" > "$tmp_marker"
  grep -Fvx "<!-- SDD input: $upstream @ $upstream_commit -->" "$repo_root/$downstream" |
    grep -Ev '^<!-- SDD input: .* -->$' >> "$tmp_marker" || true
  mv "$tmp_marker" "$repo_root/$downstream"
}

restore_state() {
  rm -f "$state"
  if [ "$had_state" = true ]; then
    cp "$backup" "$state"
  fi
  rm -f "$backup"
  if [ "$lock_acquired" = true ]; then
    rmdir "$lock"
  fi
}
trap restore_state EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

case "${1:-}" in
  plan) script=setup-plan.sh ;;
  tasks) script=setup-tasks.sh ;;
  prerequisites) script=check-prerequisites.sh ;;
  *) usage ;;
esac
shift

acquire_lock
if [ -f "$state" ]; then
  cp "$state" "$backup"
  had_state=true
fi

rm -f "$state"
SPECIFY_INIT_DIR="$repo_root" SPECIFY_FEATURE_DIRECTORY="$feature" \
  bash "$repo_root/.specify/scripts/bash/$script" "$@"

case "$script" in
  setup-plan.sh)
    record_input "$feature/spec.md" "$feature/plan.md"
    ;;
  setup-tasks.sh)
    if [ -f "$repo_root/$feature/ui-design.md" ]; then
      record_input "$feature/ui-design.md" "$feature/tasks.md"
    else
      record_input "$feature/plan.md" "$feature/tasks.md"
    fi
    ;;
esac
