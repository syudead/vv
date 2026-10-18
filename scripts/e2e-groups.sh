#!/usr/bin/env bash
# Ensure each web/e2e/*.e2e.ts appears exactly once in the CI matrix; unlisted files never run.
# Usage: scripts/e2e-groups.sh
set -eu

workflow=".github/workflows/ci.yml"

listed="$(grep -E '^[[:space:]]+files: e2e/' "$workflow" | sed -E 's/^[[:space:]]+files: //' | tr ' ' '\n' | grep -v '^$' | sort)"
present="$(cd web && ls e2e/*.e2e.ts | sort)"

status=0
duplicated="$(printf '%s\n' "$listed" | uniq -d)"
if [ -n "$duplicated" ]; then
	echo "2 つ以上の組に入っている e2e のファイルがあります:" >&2
	printf '  %s\n' $duplicated >&2
	status=1
fi
missing="$(comm -13 <(printf '%s\n' "$listed" | uniq) <(printf '%s\n' "$present"))"
if [ -n "$missing" ]; then
	echo "$workflow のどの組にも入っていない e2e のファイルがあります:" >&2
	printf '  %s\n' $missing >&2
	status=1
fi
stale="$(comm -23 <(printf '%s\n' "$listed" | uniq) <(printf '%s\n' "$present"))"
if [ -n "$stale" ]; then
	echo "$workflow の組が、無い e2e のファイルを指しています:" >&2
	printf '  %s\n' $stale >&2
	status=1
fi
if [ "$status" -eq 0 ]; then
	echo "e2e-groups: すべての e2e のファイルがちょうど 1 つの組に入っています"
fi
exit "$status"
