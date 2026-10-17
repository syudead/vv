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
# 比較対象を省いたときは origin/main を使う。配布物（Docker イメージ、Windows
# アプリ）は main からだけ作るので、既存のデータベースが適用済みでありうる版は
# main にある版である。フィーチャーブランチだけにある版はまだ配布されておらず、
# main を取り込んで番号が重なったときはフィーチャー側を新しい番号へ改める
# （PR #834 の 00036 → 00038、PR #1215 の 00048 → 00049）。PR の base branch と
# 比べると、フィーチャーブランチへの PR でこの改名を壊れた変更として報告し、
# 手元（origin/main と比べる）と CI で判定が変わる。
#
# 比べるのは比較対象の先端ではなく、HEAD との分岐点（merge-base）である。先端と
# 比べると、main がブランチの後に足した版（ブランチに無い版）を「削除」と数え、
# main を取り込んでいないフィーチャーブランチとその PR がすべて落ちていた
# （2026-10-16 の 00051〜00053、10-17 の 00055）。分岐点にある版はすべて main に
# 配布済みのものなので、それを変えていないかを見れば足りる。
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

# 内容の変更(M)だけでなく、削除(D)と改名(R)も履歴を変える。新しい番号の
# 追加(A)は許す。作業ツリーと比べるので、コミット前の変更も報告する。
modified="$(git diff --name-only --diff-filter=DMR "$fork" -- "$dir")"

if [ -n "$modified" ]; then
	# 通常の独立したコメント行だけは整理できる。SQL、goose 指示、文字列中の
	# コメントに見える行、削除・改名は引き続き変更として扱う。
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
