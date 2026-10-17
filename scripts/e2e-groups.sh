#!/usr/bin/env bash
# CI の e2e の組（.github/workflows/ci.yml の e2e ジョブの matrix）が、web/e2e の
# *.e2e.ts をちょうど 1 回ずつ持つか確かめる。
#
# CI はファイルを組に分けて並べて流し、組に名前の無いファイルは流さない。
# auto-import.e2e.ts と tag-kinds.e2e.ts は、足したときに組へ入れ忘れられ、
# CI で一度も流れていなかった。
#
# 使い方: scripts/e2e-groups.sh
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
