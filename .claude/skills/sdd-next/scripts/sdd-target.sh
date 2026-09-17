#!/usr/bin/env bash
set -euo pipefail

root="."
github_dir=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --root) root="$2"; shift 2 ;;
    --github-dir) github_dir="$2"; shift 2 ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done

fallback="$("$(dirname "$0")/sdd-state.sh" --root "$root")"
fallback_dir="$(printf '%s' "$fallback" | jq -r '.feature_dir // empty')"

[ -n "$github_dir" ] || { printf '%s\n' "$fallback_dir"; exit 0; }
closed_json="$(cat "$github_dir/pulls-closed.json")"
merged_commits="$(git -C "$root" log --first-parent --format='%H%x09%s' HEAD 2>/dev/null |
  sed -n -e 's/^\([0-9a-f]*\)\t.*Merge pull request #\([0-9][0-9]*\) .*/\2\t\1/p' \
         -e 's/^\([0-9a-f]*\)\t.*(#\([0-9][0-9]*\))$/\2\t\1/p')"
sdd_numbers="$(printf '%s' "$closed_json" | jq -r '.[] | select(([.labels[]? | if type == "object" then .name else . end] | index("sdd")) != null) | .number')"
tab="$(printf '\t')"
while IFS="$tab" read -r number commit; do
  [ -n "${number:-}" ] || continue
  printf '%s\n' "$sdd_numbers" | grep -qx "$number" || continue
  target="$(git -C "$root" diff --name-only "$commit^1" "$commit" 2>/dev/null |
    sed -n 's#^\(specs/[0-9][0-9][0-9]-[^/]*\)/.*#\1#p' | sed -n '1p')"
  if [ -n "$target" ] && [ -d "$root/$target" ]; then
    printf '%s\n' "$target"
    exit 0
  fi
done <<EOF
$merged_commits
EOF
printf '%s\n' "$fallback_dir"
