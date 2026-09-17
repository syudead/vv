#!/usr/bin/env bash
# 対象 feature の確定。
#
#   sdd-target.sh [--root <repo_root>] [--pr <番号>]
#
# `--pr` は routine を起動した PR の番号（`<github-trigger-context>` の `PR: #N`）。
# その PR のマージコミットを git 履歴から探し、対象 feature を決める。
#   - 段階 PR: 件名 "Merge pull request #N from <owner>/claude/sdd-NNN-<stage>" の NNN
#   - spec PR（人が付けた branch 名）: 差分で最も多く触った `specs/NNN-*/`
# 見つからない、または `--pr` が無い（保守者の手打ち）ときは `sdd-state.sh` の
# 自動選択（未完了の先頭）に落ちる。GitHub API は使わない。
set -euo pipefail

root="."
pr=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --root) root="$2"; shift 2 ;;
    --pr) pr="$2"; shift 2 ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done

fallback="$("$(dirname "$0")/sdd-state.sh" --root "$root")"
fallback_dir="$(printf '%s' "$fallback" | jq -r '.feature_dir // empty')"

case "$pr" in
  '') printf '%s\n' "$fallback_dir"; exit 0 ;;
  *[!0-9]*) printf 'sdd-target.sh: --pr は数字だけを受けます: %s\n' "$pr" >&2; exit 2 ;;
esac

# マージコミット "Merge pull request #N from ..." と squash "... (#N)" の両方を探す。
# feature branch を復元する前に呼ばれても見つかるよう、ローカルにある全 ref を見る。
# それでも無ければ（clone が既定 branch しか持たない場合）origin を 1 回 fetch して探し直す。
tab="$(printf '\t')"
find_merge() {
  git -C "$root" log --all --format="%H%x09%s" 2>/dev/null \
    | grep -E -m1 "^[0-9a-f]+${tab}(Merge pull request #$pr |.*\(#$pr\)$)" || true
}
line="$(find_merge)"
if [ -z "$line" ] \
  && git -C "$root" fetch -q origin '+refs/heads/*:refs/remotes/origin/*' 2>/dev/null; then
  line="$(find_merge)"
fi
commit="${line%%"$tab"*}"
subject="${line#*"$tab"}"

target=""
if [ -n "$commit" ]; then
  # 段階 PR は head 名 claude/sdd-NNN-<stage> に機能番号を持つ。これが最も確実。
  num="$(printf '%s' "$subject" \
    | sed -n 's|^Merge pull request #[0-9]* from [^/]*/claude/sdd-\([0-9][0-9][0-9]\)-.*|\1|p')"
  if [ -n "$num" ]; then
    target="$(cd "$root" && ls -d "specs/$num"-*/ 2>/dev/null | sed -n 's#/$##p' | sed -n '1p')"
  fi
  # spec PR（人が付けた branch 名）は差分から。触ったファイル数が最も多い specs/NNN-*/ を採る。
  if [ -z "$target" ]; then
    target="$(git -C "$root" diff --name-only "$commit^1" "$commit" 2>/dev/null \
      | sed -n 's#^\(specs/[0-9][0-9][0-9]-[^/]*\)/.*#\1#p' \
      | sort | uniq -c | sort -k1,1nr -k2,2r | awk 'NR==1 {print $2}')"
  fi
fi

if [ -n "$target" ] && [ -d "$root/$target" ]; then
  printf '%s\n' "$target"
  exit 0
fi
printf '%s\n' "$fallback_dir"
