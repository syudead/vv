#!/usr/bin/env bash
# sdd-state.sh / sdd-guard.sh が source して使う共通関数。
#
# 依存は bash・awk・grep・sed だけである（research.md R-007）。`jq` は使わない —
# 保守者の手元（Windows の Git Bash）には入っていないため。
#
# 読み込む行は末尾の `\r` を落としてから解釈する。手元は core.autocrlf=true なので
# フィクスチャや tasks.md が CRLF でチェックアウトされうる。

# sdd_json_escape <文字列>
#
# JSON の文字列リテラルに入れるためのエスケープ。`\` を先に置換しないと、`"` を
# `\"` にした結果のバックスラッシュを二重に escape してしまう。
# 改行とタブは対象にしない — 扱う値（phase_title）は見出しの 1 行だけで、
# 制御文字を含まないため（contracts/sdd-state.md「JSON のエスケープ」）。
sdd_json_escape() {
  local s="${1:-}"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '%s' "$s"
}

# sdd_list_features <repo_root>
#
# `<root>/specs/` 直下の `[0-9][0-9][0-9]-*` ディレクトリを名前の昇順で
# `specs/NNN-name` の相対パスとして 1 行ずつ出す。並びは LC_ALL=C で固定する
# （locale で順序が変わると、どの機能が選ばれるかが環境に依存してしまう）。
sdd_list_features() {
  local root="${1:-.}"
  [ -d "$root/specs" ] || return 0
  (
    export LC_ALL=C
    cd "$root/specs" 2>/dev/null || exit 0
    local name
    for name in [0-9][0-9][0-9]-*; do
      [ -d "$name" ] || continue
      printf 'specs/%s\n' "$name"
    done
  )
}

# sdd_phases <tasks.md>
#
# フェーズごとに `N<TAB>title<TAB>total<TAB>remaining` を 1 行ずつ出す。
# 規則は research.md R-008 のとおり:
#   - フェーズ見出しは行頭が `## Phase N:`（N は 1 以上の整数）
#   - 題名は最初の `:` の後ろ全体を前後の空白で trim したもの（`(Priority: P1)` や絵文字を含む）
#   - 節の範囲はその見出しから次の `## ` 見出し（または EOF）まで
#   - タスク行は行頭が `- [ ] `（未完了）／`- [x] ` / `- [X] `（完了）。インデントされた行は数えない
sdd_phases() {
  local file="${1:-}"
  [ -f "$file" ] || return 0
  awk '
    {
      line = $0
      sub(/\r$/, "", line)

      if (line ~ /^## Phase [0-9]+:/) {
        n++
        rest = line
        sub(/^## Phase /, "", rest)
        c = index(rest, ":")
        num[n] = substr(rest, 1, c - 1) + 0
        t = substr(rest, c + 1)
        sub(/^[ \t]+/, "", t)
        sub(/[ \t]+$/, "", t)
        title[n] = t
        total[n] = 0
        remaining[n] = 0
        cur = n
        next
      }

      # Phase 以外の `## ` 見出しは、直前のフェーズ節を閉じる。
      if (line ~ /^## /) { cur = 0; next }

      if (cur == 0) next
      if (line ~ /^- \[ \] /) { total[cur]++; remaining[cur]++; next }
      if (line ~ /^- \[[xX]\] /) { total[cur]++; next }
    }
    END {
      for (i = 1; i <= n; i++) {
        printf "%d\t%s\t%d\t%d\n", num[i], title[i], total[i], remaining[i]
      }
    }
  ' "$file"
}
