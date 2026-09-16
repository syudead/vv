#!/usr/bin/env bash
# 状態判定。`specs/` の成果物だけを読み、対象機能と次の段階を JSON 1 行で出す。
#
# 契約: specs/003-sdd-loop-harness/contracts/sdd-state.md
# 定義: specs/003-sdd-loop-harness/data-model.md 1.〜4.
#
# git にも gh にも触らない。判定に隠れた状態を持たないので、手元でそのまま実行して
# 検算できる（FR-009 / FR-010）。依存は bash・awk のみ（`jq` を使わない）。
#
# 終了コードは 0（判定できた。`none`／`done` を含む）と 2（引数の誤り）だけである。

set -u

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./sdd-lib.sh
. "$script_dir/sdd-lib.sh"

usage_error() {
  printf 'sdd-state.sh: %s\n' "$1" >&2
  printf 'usage: sdd-state.sh [--root <repo_root>] [--feature <specs/NNN-name>]\n' >&2
  exit 2
}

root="."
feature=""

while [ $# -gt 0 ]; do
  case "$1" in
    --root)
      shift
      [ $# -gt 0 ] || usage_error "--root に値がありません"
      root="$1"
      ;;
    --feature)
      shift
      [ $# -gt 0 ] || usage_error "--feature に値がありません"
      feature="$1"
      ;;
    *)
      usage_error "未知の引数: $1"
      ;;
  esac
  shift
done

[ -d "$root" ] || usage_error "--root のディレクトリがありません: $root"
if [ -n "$feature" ] && [ ! -d "$root/$feature" ]; then
  usage_error "--feature のディレクトリがありません: $root/$feature"
fi

# state_for <repo_root> <specs/NNN-name>
#
# `stage<TAB>JSON` を 1 行で出す。stage を前に付けるのは、自動選択が JSON を
# 文字列検索せずに `none`／`done` を飛ばせるようにするためである。
state_for() {
  local root="$1" fdir="$2"
  local abs="$root/$fdir"
  local name num
  name="$(basename "$fdir")"
  num="${name:0:3}"

  if [ ! -f "$abs/spec.md" ]; then
    printf 'none\t{"feature_dir":"%s","feature":"%s","stage":"none"}\n' "$fdir" "$num"
    return
  fi
  if [ ! -f "$abs/plan.md" ]; then
    printf 'plan\t{"feature_dir":"%s","feature":"%s","stage":"plan","feature_branch":"claude/sdd-%s-feature","base_branch":"claude/sdd-%s-feature","branch":"claude/sdd-%s-plan"}\n' \
      "$fdir" "$num" "$num" "$num" "$num"
    return
  fi
  if [ ! -f "$abs/tasks.md" ]; then
    printf 'tasks\t{"feature_dir":"%s","feature":"%s","stage":"tasks","feature_branch":"claude/sdd-%s-feature","base_branch":"claude/sdd-%s-feature","branch":"claude/sdd-%s-tasks"}\n' \
      "$fdir" "$num" "$num" "$num" "$num"
    return
  fi

  local phases=0 sel_num="" sel_title="" sel_total=0 sel_remaining=0
  local p_num p_title p_total p_remaining
  while IFS="$(printf '\t')" read -r p_num p_title p_total p_remaining; do
    [ -n "${p_num:-}" ] || continue
    phases=$((phases + 1))
    # 対象フェーズは「remaining > 0 の最初のフェーズ」（data-model.md 3.）。
    if [ -z "$sel_num" ] && [ "${p_remaining:-0}" -gt 0 ]; then
      sel_num="$p_num"
      sel_title="$p_title"
      sel_total="$p_total"
      sel_remaining="$p_remaining"
    fi
  done < <(sdd_phases "$abs/tasks.md")

  if [ -z "$sel_num" ]; then
    printf 'done\t{"feature_dir":"%s","feature":"%s","stage":"done","phases":%d,"feature_branch":"claude/sdd-%s-feature","base_branch":"main"}\n' \
      "$fdir" "$num" "$phases" "$num"
    return
  fi

  local esc
  esc="$(sdd_json_escape "$sel_title")"
  printf 'implement\t{"feature_dir":"%s","feature":"%s","stage":"implement","phase":%d,"phase_title":"%s","remaining":%d,"total":%d,"phases":%d,"feature_branch":"claude/sdd-%s-feature","base_branch":"claude/sdd-%s-feature","branch":"claude/sdd-%s-implement-p%d"}\n' \
    "$fdir" "$num" "$sel_num" "$esc" "$sel_remaining" "$sel_total" "$phases" \
    "$num" "$num" "$num" "$sel_num"
}

tab="$(printf '\t')"

# --feature が与えられていれば、その機能を `none`／`done` でもそのまま判定する。
if [ -n "$feature" ]; then
  line="$(state_for "$root" "${feature%/}")"
  printf '%s\n' "${line#*$tab}"
  exit 0
fi

# 自動選択: 名前の昇順に判定し、`none` でも `done` でもない最初の機能を返す。
selected=""
while IFS= read -r fdir; do
  [ -n "$fdir" ] || continue
  line="$(state_for "$root" "$fdir")"
  stage="${line%%$tab*}"
  case "$stage" in
    none|done) continue ;;
  esac
  selected="${line#*$tab}"
  break
done < <(sdd_list_features "$root")

if [ -n "$selected" ]; then
  printf '%s\n' "$selected"
else
  printf '%s\n' '{"stage":"none"}'
fi
exit 0
