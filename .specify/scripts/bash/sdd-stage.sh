#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'usage: sdd-stage.sh --feature specs/NNN-name [--ui] [--root PATH]\n' >&2
  exit 2
}

feature=""
workflow="standard"
root=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --feature)
      [ "$#" -ge 2 ] || usage
      feature=$2
      shift 2
      ;;
    --ui)
      workflow="ui"
      shift
      ;;
    --root)
      [ "$#" -ge 2 ] || usage
      root=$2
      shift 2
      ;;
    *) usage ;;
  esac
done

[ -n "$feature" ] || usage
case "$feature" in
  specs/*) ;;
  *) printf 'sdd-stage: feature must be under specs/: %s\n' "$feature" >&2; exit 2 ;;
esac
case "/$feature/" in
  */../*|*/./*) printf 'sdd-stage: feature path must be normalized: %s\n' "$feature" >&2; exit 2 ;;
esac

if [ -z "$root" ]; then
  root=$(git rev-parse --show-toplevel 2>/dev/null) || {
    printf 'sdd-stage: not inside a git repository\n' >&2
    exit 2
  }
fi
root=$(cd "$root" && pwd)
dir="$root/$feature"
[ -d "$dir" ] || {
  printf 'sdd-stage: feature directory does not exist: %s\n' "$feature" >&2
  exit 2
}

emit() {
  printf 'feature=%s\nworkflow=%s\nnext=%s\nreason=%s\n' \
    "$feature" "$workflow" "$1" "$2"
  if [ -n "${parent_issue:-}" ]; then
    printf 'parent_issue=%s\n' "$parent_issue"
  fi
  exit 0
}

spec="$feature/spec.md"
plan="$feature/plan.md"
design="$feature/ui-design.md"
tasks="$feature/tasks.md"

if [ ! -f "$root/$spec" ]; then
  emit "specify" "missing spec.md"
fi

mapfile -t parent_lines < <(tr -d '\r' < "$root/$spec" | sed -n 's/^\*\*Parent Issue\*\*: #\([1-9][0-9]*\)$/\1/p')
if [ "${#parent_lines[@]}" -ne 1 ]; then
  printf 'sdd-stage: spec.md must contain exactly one **Parent Issue**: #NNN line\n' >&2
  exit 2
fi
parent_issue=${parent_lines[0]}

if [ -n "$(git -C "$root" status --porcelain -- "$feature")" ]; then
  printf 'sdd-stage: feature artifacts must be clean before state is evaluated\n' >&2
  exit 2
fi

latest_commit() {
  git -C "$root" log -1 --format=%H -- "$1"
}

includes_latest() {
  local upstream=$1 downstream=$2 upstream_commit downstream_commit
  upstream_commit=$(latest_commit "$upstream")
  downstream_commit=$(latest_commit "$downstream")
  [ -n "$upstream_commit" ] && [ -n "$downstream_commit" ] &&
    git -C "$root" merge-base --is-ancestor "$upstream_commit" "$downstream_commit"
}

if [ ! -f "$root/$plan" ]; then
  emit "plan" "missing plan.md"
fi
if ! includes_latest "$spec" "$plan"; then
  emit "plan" "plan.md does not include the latest spec.md"
fi

tasks_input=$plan
if [ "$workflow" = "ui" ]; then
  if [ ! -f "$root/$design" ]; then
    emit "design" "missing ui-design.md"
  fi
  if ! includes_latest "$plan" "$design"; then
    emit "design" "ui-design.md does not include the latest plan.md"
  fi
  tasks_input=$design
fi

if [ ! -f "$root/$tasks" ]; then
  emit "tasks" "missing tasks.md"
fi
if ! includes_latest "$tasks_input" "$tasks"; then
  if [ "$workflow" = "ui" ]; then
    emit "tasks" "tasks.md does not include the latest ui-design.md"
  fi
  emit "tasks" "tasks.md does not include the latest plan.md"
fi

emit "taskstoissues" "artifacts are current"
