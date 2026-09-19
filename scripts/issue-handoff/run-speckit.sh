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
backup=$(mktemp)
had_state=false

if [ -f "$state" ]; then
  cp "$state" "$backup"
  had_state=true
fi

restore_state() {
  rm -f "$state"
  if [ "$had_state" = true ]; then
    cp "$backup" "$state"
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

rm -f "$state"
SPECIFY_INIT_DIR="$repo_root" SPECIFY_FEATURE_DIRECTORY="$feature" \
  bash "$repo_root/.specify/scripts/bash/$script" "$@"
