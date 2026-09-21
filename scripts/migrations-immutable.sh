#!/usr/bin/env bash
# 適用済みのマイグレーションを書き換えていないか確認する。
#
# goose は適用済みの版を版番号で管理するので、既に配布した .sql を書き換えても
# 再実行されない。新しい列を古いマイグレーションへ足すと、新規DBだけが正しく、
# 既に更新済みのDBは列を持たないまま起動して落ちる。PR #80 がこれで、
# 00003 を書き換えたあと 00004 を新設して直している。
#
# 使い方: scripts/migrations-immutable.sh [比較対象]
#
# 比較対象を省いたときは、PR の base branch（GITHUB_BASE_REF）があればそれを、
# 無ければ origin/main を使う。呼び出し側が渡し分けると、手元と CI で違う
# 比較になり、同じコマンドを実行しても判定が変わる。
set -eu

base="${1:-}"
if [ -z "$base" ]; then
	if [ -n "${GITHUB_BASE_REF:-}" ]; then
		base="origin/${GITHUB_BASE_REF}"
	else
		base="origin/main"
	fi
fi
dir="internal/store/migrations"

if ! git rev-parse --verify --quiet "$base" >/dev/null; then
	echo "比較対象 $base を解決できません。git fetch してから実行してください。" >&2
	exit 2
fi

# 内容の変更(M)だけでなく、削除(D)と改名(R)も履歴を変える。新しい番号の
# 追加(A)は許す。
modified="$(git diff --name-only --diff-filter=DMR "$base" -- "$dir")"

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
