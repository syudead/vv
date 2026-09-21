#!/usr/bin/env bash
# 適用済みのマイグレーションを書き換えていないか確認する。
#
# goose は適用済みの版を版番号で管理するので、既に配布した .sql を書き換えても
# 再実行されない。新しい列を古いマイグレーションへ足すと、新規DBだけが正しく、
# 既に更新済みのDBは列を持たないまま起動して落ちる。PR #80 がこれで、
# 00003 を書き換えたあと 00004 を新設して直している。
#
# 使い方: scripts/migrations-immutable.sh [比較対象]
#   比較対象の既定は origin/main。
set -eu

base="${1:-origin/main}"
dir="internal/store/migrations"

if ! git rev-parse --verify --quiet "$base" >/dev/null; then
	echo "比較対象 $base を解決できません。git fetch してから実行してください。" >&2
	exit 2
fi

modified="$(git diff --name-only --diff-filter=M "$base" -- "$dir")"

if [ -n "$modified" ]; then
	echo "適用済みのマイグレーションが変更されています:" >&2
	echo "$modified" | sed 's/^/  /' >&2
	cat >&2 <<'MSG'

goose は適用済みの版を再実行しないため、この変更は既存のデータベースへ届きません。
新規のデータベースだけが新しい定義になり、更新済みのデータベースは古いままで起動に
失敗します。変更を元に戻し、新しい番号のマイグレーションを追加してください。
MSG
	exit 1
fi

echo "migrations-immutable: $base に対する変更はありません"
