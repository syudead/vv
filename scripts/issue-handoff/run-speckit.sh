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
    owner_pid=$(cat "$lock/owner" 2>/dev/null || true)
    if [[ "$owner_pid" =~ ^[1-9][0-9]*$ ]] && ! kill -0 "$owner_pid" 2>/dev/null; then
      rm -f "$lock/owner"
      rmdir "$lock" 2>/dev/null || true
      continue
    fi
    sleep 1
  done
  printf '%s\n' "$$" > "$lock/owner"
  lock_acquired=true
}

restore_state() {
  if [ "$lock_acquired" = true ]; then
    rm -f "$state"
    if [ "$had_state" = true ]; then
      cp "$backup" "$state"
    fi
    rm -f "$lock/owner"
    rmdir "$lock"
  fi
  rm -f "$backup"
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
