#!/usr/bin/env bash
# ガード。stdin で受けた State に対し、GitHub の PR を見て「進めてよいか」を判定する。
#
# 契約: specs/003-sdd-loop-harness/contracts/sdd-guard.md
# 定義: specs/003-sdd-loop-harness/data-model.md 5.〜6.
#
# GitHub から要るのは base を限定しない closed PR 一覧と open PR 一覧の 2 つだけで、
# 取得手段は 2 通りある。
#   a) --github-dir <dir>: <dir>/pulls-closed.json と <dir>/pulls-open.json を読む。
#      cloud セッションには `gh` が無いので、スキルが組み込みの GitHub ツールで取った結果を
#      ファイルに置いてから呼ぶ（research.md R-002）
#   b) 指定が無ければ `gh api` の REST で取る（手元の検算用）。`gh pr list` などの高水準
#      コマンドは GraphQL を使い、プロキシが 403 を返しうるので使わない
#
# 「マージ済みか」は API の merged_at ではなく、作業 base の git 履歴（HEAD の first-parent に
# `Merge pull request #N` か `(#N)` があるか）で決める。取得元によって PR オブジェクトの
# 項目が違っても判定が変わらないようにするためで、変更ファイルも同じ履歴から取る。
#
# `jq` が無い、`gh` が無い、または REST が通らない場合も終了コード 0 で
# `{"go":false,"reason":"gh-unavailable","state":<stdin そのまま>}` を返す。手元
# （Windows の Git Bash、`gh`・`jq` 無し）でも検算できるようにするためである。

set -u

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage_error() {
  printf 'sdd-guard.sh: %s\n' "$1" >&2
  printf 'usage: sdd-state.sh | sdd-guard.sh [--repo <owner/name>] [--root <repo_root>] [--github-dir <dir>]\n' >&2
  exit 2
}

repo=""
root="."
github_dir=""

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
    --github-dir)
      shift
      [ $# -gt 0 ] || usage_error "--github-dir に値がありません"
      github_dir="$1"
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

# --- 手順 0: PR 一覧の取得 ----------------------------------------------------
command -v jq >/dev/null 2>&1 || emit_unavailable

if [ -n "$github_dir" ]; then
  [ -r "$github_dir/pulls-closed.json" ] || usage_error "$github_dir/pulls-closed.json が読めません"
  [ -r "$github_dir/pulls-open.json" ] || usage_error "$github_dir/pulls-open.json が読めません"
  closed_json="$(cat "$github_dir/pulls-closed.json")"
  open_json="$(cat "$github_dir/pulls-open.json")"
  printf '%s' "$closed_json" | jq -e 'type == "array"' >/dev/null 2>&1 \
    || usage_error "$github_dir/pulls-closed.json が JSON の配列ではありません"
  printf '%s' "$open_json" | jq -e 'type == "array"' >/dev/null 2>&1 \
    || usage_error "$github_dir/pulls-open.json が JSON の配列ではありません"
else
  command -v gh >/dev/null 2>&1 || emit_unavailable
  gh api /rate_limit >/dev/null 2>&1 || emit_unavailable

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

  fetch_pulls() {
    local pull_state="$1" page=1 page_json all_json='[]'
    while :; do
      if ! page_json="$(gh api "/repos/$owner/$name/pulls?state=$pull_state&sort=updated&direction=desc&per_page=100&page=$page" 2>/dev/null)"; then
        emit_unavailable
      fi
      printf '%s' "$page_json" | jq -e 'type == "array"' >/dev/null 2>&1 || emit_unavailable
      all_json="$(printf '%s\n%s\n' "$all_json" "$page_json" | jq -s 'add')"
      count="$(printf '%s' "$page_json" | jq -r 'length')"
      [ "${count:-0}" -eq 100 ] || break
      page=$((page + 1))
    done
    printf '%s' "$all_json"
  }
  # 段階 PR は feature branch 向け、最終 PR は main 向けなので base を絞らない。
  closed_json="$(fetch_pulls closed)"
  open_json="$(fetch_pulls open)"
fi

# --- マージ済み PR を git 履歴から並べる ---------------------------------------
# HEAD の first-parent を新しい順に辿り、件名から PR 番号を取る。
#   マージコミット: "Merge pull request #N from ..."  /  squash: "... (#N)"
# 出力は "<番号>\t<コミット>" を新しい順に並べたもの。
merged_commits="$(git -C "$root" log --first-parent --format='%H%x09%s' HEAD 2>/dev/null \
  | sed -n \
      -e 's/^\([0-9a-f]*\)\t.*Merge pull request #\([0-9][0-9]*\) .*/\2\t\1/p' \
      -e 's/^\([0-9a-f]*\)\t.*(#\([0-9][0-9]*\))$/\2\t\1/p')"

# --- 手順 1: 対象機能の確定 -------------------------------------------------
# closed 一覧のうち `sdd` ラベル付きで、かつ HEAD の履歴にマージされているものが「マージ済み
# sdd PR」である。その直近 1 件が触った `specs/NNN-*/` を、今回の対象とみなす。
# labels は REST では `[{"name":"sdd"}]`、組み込み GitHub ツールでは `["sdd"]` で届くので、
# 両方を受ける。
sdd_closed="$(printf '%s' "$closed_json" \
  | jq -r '.[] | select(([.labels[]? | if type == "object" then .name else . end]
                         | index("sdd")) != null)
                | "\(.number)\t\(.head.ref)"' 2>/dev/null || printf '')"

tab="$(printf '\t')"

# merged_list: "<番号>\t<head.ref>" を新しい順に。マージ済み sdd PR だけ。
merged_list=""
latest_commit=""
while IFS="$tab" read -r num commit; do
  [ -n "${num:-}" ] || continue
  ref="$(printf '%s\n' "$sdd_closed" | sed -n "s/^$num$tab//p" | sed -n '1p')"
  [ -n "$ref" ] || continue
  merged_list="${merged_list}${num}${tab}${ref}
"
  [ -n "$latest_commit" ] || latest_commit="$commit"
done <<MERGED
$merged_commits
MERGED

if [ -n "$latest_commit" ]; then
  target_dir="$(git -C "$root" diff --name-only "$latest_commit^1" "$latest_commit" 2>/dev/null \
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
feature_branch="claude/sdd-$feature-feature"

# --- ホップの集計（手順 1 で作った一覧を使い回す） --------------------------
hops=0
phase_retries=0
phase_ref="claude/sdd-$feature-implement-p$phase"
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
open_prs="$(printf '%s' "$open_json" \
  | jq -c --arg p "$prefix" --arg b "$feature_branch" \
      '[.[] | select(.base.ref == $b)
              | select(([.labels[]? | if type == "object" then .name else . end]
                        | index("sdd")) != null)
              | .head.ref | select(startswith($p))]' \
      2>/dev/null || printf '[]')"
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
