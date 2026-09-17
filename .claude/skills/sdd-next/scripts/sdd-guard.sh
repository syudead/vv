#!/usr/bin/env bash
# ガード。stdin で受けた State に対し、git だけを見て「進めてよいか」を判定する。
#
# 契約: specs/003-sdd-loop-harness/contracts/sdd-guard.md
# 定義: specs/003-sdd-loop-harness/data-model.md 5.〜6.
#
# GitHub API は使わない。要る情報はすべて git に載っている。
#   - マージ済み段階 PR の head 名: HEAD の first-parent にある merge commit の件名
#     "Merge pull request #N from <owner>/<head.ref>"
#   - 進行中の段階 PR:           remote に残る `claude/sdd-NNN-*` の branch
#     （リポジトリは delete_branch_on_merge なので、残っている = まだマージされていない）
#     マージせず閉じた PR の branch は消えないので、その回復（branch 削除 → やり直し）は
#     スキルが closed 未マージ PR の実在を確かめてから行う（SKILL.md 1. の open-pr）
#   - feature branch の有無:      同じ ls-remote の結果
#
# 以前は組み込み GitHub ツールで取った PR 一覧を `--github-dir` で渡していたが、応答が
# 大きいとツール結果が `~/.claude/projects/` 配下にスプールされ、それを Bash で触った時点で
# sandbox の権限プロンプトに掛かって routine が止まった（2026-09-16、
# session_01E9LfqSkmUk9ke8owLbm9Nh）。git 専用にしてその経路を無くす。
#
# squash マージ（件名 "... (#N)"）は head 名が分からないので hop に数えない。段階 PR は
# merge commit でマージすること（SKILL.md 5.）。人が plan PR を squash しても、hop 上限が
# 少し緩くなるだけで判定は壊れない。
#
# `git ls-remote` が失敗したら終了コード 0 で
# `{"go":false,"reason":"remote-unavailable","state":<stdin そのまま>}` を返す。

set -u

usage_error() {
  printf 'sdd-guard.sh: %s\n' "$1" >&2
  printf 'usage: sdd-state.sh | sdd-guard.sh [--root <repo_root>] [--remote <name>]\n' >&2
  exit 2
}

root="."
remote="origin"

while [ $# -gt 0 ]; do
  case "$1" in
    --root)
      shift
      [ $# -gt 0 ] || usage_error "--root に値がありません"
      root="$1"
      ;;
    --remote)
      shift
      [ $# -gt 0 ] || usage_error "--remote に値がありません"
      remote="$1"
      ;;
    *)
      usage_error "未知の引数: $1"
      ;;
  esac
  shift
done

command -v jq >/dev/null 2>&1 || usage_error "jq が必要です（mise install）"
command -v git >/dev/null 2>&1 || usage_error "git が必要です"

# stdin は sdd-state.sh の出力（JSON 1 行）。CR と改行を落として 1 行に正規化する。
state="$(cat)"
state="$(printf '%s' "$state" | tr -d '\r\n')"
case "$state" in
  '{'*) ;;
  *) usage_error "stdin が JSON ではありません" ;;
esac

json_field() { printf '%s' "$state" | jq -r "$1" 2>/dev/null || printf ''; }

stage="$(json_field '.stage // "none"')"
feature="$(json_field '.feature // ""')"
phases="$(json_field '.phases // 0')"
phase="$(json_field '.phase // 0')"
workflow="$(json_field '.workflow // "standard"')"

# --- 手順 1: 進める先が無い -------------------------------------------------
if [ "$stage" = "done" ] || [ "$stage" = "none" ] || [ -z "$feature" ]; then
  printf '{"go":false,"reason":"nothing-to-do","state":%s}\n' "$state"
  exit 0
fi

prefix="claude/sdd-$feature-"
feature_branch="claude/sdd-$feature-feature"

# --- 手順 2: remote に残る段階 branch と feature branch --------------------
# 出力は "<sha>\trefs/heads/<name>" の行。失敗（remote 無し・ネットワーク不可）は fail-closed。
if ! remote_refs="$(git -C "$root" ls-remote --heads "$remote" "refs/heads/$prefix*" 2>/dev/null)"; then
  printf '{"go":false,"reason":"remote-unavailable","state":%s}\n' "$state"
  exit 0
fi
remote_heads="$(printf '%s\n' "$remote_refs" | sed -n 's#^[0-9a-f]*[[:space:]]*refs/heads/##p')"

feature_exists=false
open_prs='[]'
while IFS= read -r ref; do
  [ -n "$ref" ] || continue
  if [ "$ref" = "$feature_branch" ]; then
    feature_exists=true
  else
    open_prs="$(printf '%s' "$open_prs" | jq -c --arg r "$ref" '. + [$r]')"
  fi
done <<REFS
$remote_heads
REFS

# --- 手順 3: 作業 base の確認 -----------------------------------------------
# feature branch が remote にあるなら、段階の履歴はそこにしかない。main のまま判定すると
# 「plan からやり直せ」に見えてしまうので、feature branch 上でなければ止める。
current_branch="$(git -C "$root" branch --show-current 2>/dev/null || printf '')"
if [ "$feature_exists" = true ] && [ "$current_branch" != "$feature_branch" ]; then
  printf '{"go":false,"reason":"wrong-base","state":%s,"feature_branch":"%s","current_branch":"%s"}\n' \
    "$state" "$feature_branch" "$current_branch"
  exit 0
fi

# --- ホップの集計: HEAD の first-parent にある merge commit の件名 ------------
# "Merge pull request #N from <owner>/<head.ref>" から head.ref を取る。
merged_refs="$(git -C "$root" log --first-parent --format='%s' HEAD 2>/dev/null \
  | sed -n 's/^Merge pull request #[0-9][0-9]* from [^/]*\/\(.*\)$/\1/p')"

hops=0
phase_retries=0
phase_ref="claude/sdd-$feature-implement-p$phase"
while IFS= read -r ref; do
  [ -n "$ref" ] || continue
  case "$ref" in
    "$prefix"*) hops=$((hops + 1)) ;;
    *) continue ;;
  esac
  [ "$ref" = "$phase_ref" ] && phase_retries=$((phase_retries + 1))
done <<MERGED
$merged_refs
MERGED

# --- 手順 4: 冪等（段階 branch が remote に残っていれば何もしない） ------------
open_count="$(printf '%s' "$open_prs" | jq -r 'length')"
if [ "${open_count:-0}" -gt 0 ]; then
  printf '{"go":false,"reason":"open-pr","state":%s,"hops":%d,"phase_retries":%d,"open_prs":%s}\n' \
    "$state" "$hops" "$phase_retries" "$open_prs"
  exit 0
fi

# --- 手順 5: 同じフェーズのマージが 2 回に達した ----------------------------
if [ "$stage" = "implement" ] && [ "$phase_retries" -ge 2 ]; then
  printf '{"go":false,"reason":"phase-retry-limit","state":%s,"hops":%d,"phase_retries":%d,"open_prs":[]}\n' \
    "$state" "$hops" "$phase_retries"
  exit 0
fi

# --- 手順 6: 機能あたりのホップ上限（plan + [design] + tasks + 全フェーズ + 予備 2） ---
design_hops=0
[ "$workflow" = "ui" ] && design_hops=1
hop_limit=$((2 + design_hops + phases + 2))
if [ "$hops" -ge "$hop_limit" ]; then
  printf '{"go":false,"reason":"hop-limit","state":%s,"hops":%d,"phase_retries":%d,"open_prs":[]}\n' \
    "$state" "$hops" "$phase_retries"
  exit 0
fi

# --- 手順 7: 進めてよい -----------------------------------------------------
printf '{"go":true,"state":%s,"hops":%d,"phase_retries":%d,"open_prs":[]}\n' \
  "$state" "$hops" "$phase_retries"
exit 0
