#!/usr/bin/env bash
# Reject edits to released migrations: goose tracks versions and will not rerun changed SQL.
# Compare against the merge base with origin/main (or the supplied ref), since only main ships releases and later main additions are not branch deletions.
# Usage: scripts/migrations-immutable.sh [base]
set -eu

base="${1:-${MIGRATIONS_BASE:-origin/main}}"
dir="internal/store/migrations"

if ! git rev-parse --verify --quiet "$base" >/dev/null; then
	echo "比較対象 $base を解決できません。git fetch してから実行してください。" >&2
	exit 2
fi

if ! fork="$(git merge-base "$base" HEAD)"; then
	echo "$base と HEAD の分岐点を求められません。浅い clone なら履歴を取り寄せてください。" >&2
	exit 2
fi

# Include edits, deletions and renames against the working tree; allow new migrations.
modified="$(git diff --name-only --diff-filter=DMR "$fork" -- "$dir")"

if [ -n "$modified" ]; then
	# Allow only ordinary line-comment edits; SQL, goose directives, quoted text, deletions and renames remain immutable.
	modified="$(printf '%s\n' "$modified" | go run ./scripts/migrationcomments "$fork")"
fi

if [ -n "$modified" ]; then
	echo "適用済みのマイグレーションが変更・削除されています:" >&2
	echo "$modified" | sed 's/^/  /' >&2
	cat >&2 <<'MSG'

goose は適用済みの版を再実行しないため、この変更は既存のデータベースへ届きません。
新規のデータベースだけが新しい定義になり、更新済みのデータベースは古いままで起動に
失敗します。変更を元に戻し、新しい番号のマイグレーションを追加してください。
MSG
	exit 1
fi

echo "migrations-immutable: $base に対する変更はありません"
