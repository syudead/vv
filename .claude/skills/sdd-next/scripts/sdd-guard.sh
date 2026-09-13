#!/usr/bin/env bash
# ガード。stdin で受けた State に対し、GitHub の PR を見て「進めてよいか」を判定する。
#
# 契約: specs/003-sdd-loop-harness/contracts/sdd-guard.md
# 定義: specs/003-sdd-loop-harness/data-model.md 5.〜6.
#
# GitHub へは `gh api` の REST だけで問い合わせる。`gh pr list` などの高水準コマンドは
# 内部で GraphQL を使い、cloud セッションのプロキシが 403 を返しうるため（research.md R-002）。
#
# `gh` か `jq` が無い、または REST が通らない場合も終了コード 0 で
# `{"go":false,"reason":"gh-unavailable","state":<stdin そのまま>}` を返す。手元
# （Windows の Git Bash、`gh`・`jq` 無し）でも検算できるようにするためである。

set -u

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage_error() {
  printf 'sdd-guard.sh: %s\n' "$1" >&2
  printf 'usage: sdd-state.sh | sdd-guard.sh [--repo <owner/name>] [--root <repo_root>]\n' >&2
  exit 2
}

repo=""
root="."

while [ $# -gt 0 ]; do
  case "$1" in
    --repo)
      shift
      [ $# -gt 0 ] || usage_error "--repo に値がありません"
      repo="$1"
      ;;
    --root)
      shift
      [ $# -gt 0 ] || usage_error "--root に値がありません"
      root="$1"
      ;;
    *)
      usage_error "未知の引数: $1"
      ;;
  esac
  shift
done

# stdin は sdd-state.sh の出力（JSON 1 行）。CR と改行を落として 1 行に正規化する。
state="$(cat)"
state="$(printf '%s' "$state" | tr -d '\r\n')"
case "$state" in
  '{'*) ;;
  *) usage_error "stdin が JSON ではありません" ;;
esac

# stdin をそのまま state に載せて返す。`jq` が無い場合でも使えるよう printf で組み立てる。
emit_unavailable() {
  printf '{"go":false,"reason":"gh-unavailable","state":%s}\n' "$state"
  exit 0
}

# --- 手順 0: 疎通 -----------------------------------------------------------
command -v gh >/dev/null 2>&1 || emit_unavailable
command -v jq >/dev/null 2>&1 || emit_unavailable
gh api /rate_limit >/dev/null 2>&1 || emit_unavailable

# --- リポジトリの導出 -------------------------------------------------------
if [ -z "$repo" ]; then
  url="$(git -C "$root" remote get-url origin 2>/dev/null || printf '')"
  case "$url" in
    git@github.com:*) repo="${url#git@github.com:}" ;;
    https://github.com/*) repo="${url#https://github.com/}" ;;
    ssh://git@github.com/*) repo="${url#ssh://git@github.com/}" ;;
    *) repo="" ;;
  esac
  repo="${repo%.git}"
fi
[ -n "$repo" ] || usage_error "--repo を導出できません（origin の URL: ${url:-なし}）"
owner="${repo%%/*}"
name="${repo#*/}"

api() { gh api "$1" 2>/dev/null || printf '[]'; }

# --- 手順 1: 対象機能の確定 -------------------------------------------------
# 直近のマージ済み `sdd` PR が触った `specs/NNN-*/` を、今回の対象とみなす。
# 一覧は更新日時の降順なので、先頭が直近である。
merged_json="$(api "/repos/$owner/$name/pulls?state=closed&base=main&sort=updated&direction=desc&per_page=100")"
merged_list="$(printf '%s' "$merged_json" \
  | jq -r '.[] | select(.merged_at != null)
                | select(([.labels[].name] | index("sdd")) != null)
                | "\(.number)\t\(.head.ref)"' 2>/dev/null || printf '')"

latest_num="$(printf '%s\n' "$merged_list" | sed -n '1s/\t.*//p')"
if [ -n "$latest_num" ]; then
  files_json="$(api "/repos/$owner/$name/pulls/$latest_num/files?per_page=100")"
  target_dir="$(printf '%s' "$files_json" \
    | jq -r '.[].filename' 2>/dev/null \
    | sed -n 's#^\(specs/[0-9][0-9][0-9]-[^/]*\)/.*#\1#p' \
    | sed -n '1p')"
  if [ -n "${target_dir:-}" ]; then
    # 手元にその機能ディレクトリが無ければ（終了コード 2）、stdin の state のままにする。
    if new_state="$("$script_dir/sdd-state.sh" --root "$root" --feature "$target_dir" 2>/dev/null)"; then
      state="$(printf '%s' "$new_state" | tr -d '\r\n')"
    fi
  fi
fi

json_field() { printf '%s' "$state" | jq -r "$1" 2>/dev/null || printf ''; }

stage="$(json_field '.stage // "none"')"
feature="$(json_field '.feature // ""')"
phases="$(json_field '.phases // 0')"
phase="$(json_field '.phase // 0')"

# --- 手順 2: 進める先が無い -------------------------------------------------
if [ "$stage" = "done" ] || [ "$stage" = "none" ] || [ -z "$feature" ]; then
  printf '{"go":false,"reason":"nothing-to-do","state":%s}\n' "$state"
  exit 0
fi

prefix="claude/sdd-$feature-"

# --- ホップの集計（手順 1 で取得済みの一覧を使い回す） ----------------------
hops=0
phase_retries=0
phase_ref="claude/sdd-$feature-implement-p$phase"
tab="$(printf '\t')"
while IFS="$tab" read -r _num ref; do
  [ -n "${ref:-}" ] || continue
  case "$ref" in
    "$prefix"*) hops=$((hops + 1)) ;;
  esac
  [ "$ref" = "$phase_ref" ] && phase_retries=$((phase_retries + 1))
done <<MERGED
$merged_list
MERGED

# --- 手順 3: 冪等（open な自動 PR があれば何もしない） ----------------------
open_json="$(api "/repos/$owner/$name/pulls?state=open&base=main&per_page=100")"
open_prs="$(printf '%s' "$open_json" \
  | jq -c --arg p "$prefix" '[.[].head.ref | select(startswith($p))]' 2>/dev/null || printf '[]')"
[ -n "$open_prs" ] || open_prs='[]'
open_count="$(printf '%s' "$open_prs" | jq -r 'length' 2>/dev/null || printf '0')"

if [ "${open_count:-0}" -gt 0 ]; then
  printf '{"go":false,"reason":"open-pr","state":%s,"hops":%d,"phase_retries":%d,"open_prs":%s}\n' \
    "$state" "$hops" "$phase_retries" "$open_prs"
  exit 0
fi

# --- 手順 4: 同じフェーズのマージが 2 回に達した ----------------------------
if [ "$stage" = "implement" ] && [ "$phase_retries" -ge 2 ]; then
  printf '{"go":false,"reason":"phase-retry-limit","state":%s,"hops":%d,"phase_retries":%d,"open_prs":[]}\n' \
    "$state" "$hops" "$phase_retries"
  exit 0
fi

# --- 手順 5: 機能あたりのホップ上限（plan + tasks + 全フェーズ + 予備 2） ---
hop_limit=$((2 + phases + 2))
if [ "$hops" -ge "$hop_limit" ]; then
  printf '{"go":false,"reason":"hop-limit","state":%s,"hops":%d,"phase_retries":%d,"open_prs":[]}\n' \
    "$state" "$hops" "$phase_retries"
  exit 0
fi

# --- 手順 6: 進めてよい -----------------------------------------------------
printf '{"go":true,"state":%s,"hops":%d,"phase_retries":%d,"open_prs":[]}\n' \
  "$state" "$hops" "$phase_retries"
exit 0
