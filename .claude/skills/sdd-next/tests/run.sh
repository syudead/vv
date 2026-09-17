#!/usr/bin/env bash
# SDD ハーネスの判定テスト。
#
# 依存は bash・coreutils（mktemp・tr・cat）・jq・git。`gh` は要らない
# （research.md R-007 / US3: 保守者の手元 = Windows の Git Bash で検算できること）。
#
# 検査するもの:
#   (1) fixtures/*/  … sdd-state.sh --root <fixture> の出力が expected.json と一致
#   (2) feature.txt を持つフィクスチャ … --feature でも expected-feature.json と一致
#   (3) 決定性     … 同じ入力で 2 回実行して同じ出力
#   (4) 異常系     … 引数の誤りで終了コード 2
#   (5) ガード     … gh が使えないとき sdd-guard.sh が gh-unavailable を返し state を素通しする
#
# 出力は PASS/FAIL の 1 行ずつ。失敗が 1 つでもあれば終了コード 1。

set -u

dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS="$dir/../scripts"
FIXTURES="$dir/fixtures"
STATE="$SCRIPTS/sdd-state.sh"
GUARD="$SCRIPTS/sdd-guard.sh"
UI_CLASSIFY="$SCRIPTS/sdd-ui-classify.sh"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

pass_count=0
fail_count=0

# CR と改行をすべて落として 1 行にする。フィクスチャが CRLF でチェックアウトされていても
# 判定が変わらないようにするため（手元は Windows で core.autocrlf=true）。
norm() { printf '%s' "${1:-}" | tr -d '\r\n'; }

ok() {
  pass_count=$((pass_count + 1))
  printf 'PASS %s\n' "$1"
}

ng() {
  fail_count=$((fail_count + 1))
  printf 'FAIL %s\n' "$1"
  shift
  local line
  for line in "$@"; do
    printf '     %s\n' "$line"
  done
}

# check_state <名前> <期待 JSON ファイル> <sdd-state.sh の引数...>
check_state() {
  local name="$1" expected_file="$2"
  shift 2
  local expected actual code stderr_file stderr_text
  expected="$(norm "$(cat "$expected_file" 2>/dev/null)")"
  stderr_file="$tmpdir/stderr.$$"
  actual="$("$STATE" "$@" 2>"$stderr_file")"
  code=$?
  actual="$(norm "$actual")"
  stderr_text="$(cat "$stderr_file")"
  rm -f "$stderr_file"

  if [ "$code" -ne 0 ]; then
    ng "$name" "終了コード: $code（期待: 0）" "stderr: $stderr_text" "actual:   $actual"
    return
  fi
  if [ -n "$stderr_text" ]; then
    ng "$name" "stderr が空ではない: $stderr_text"
    return
  fi
  if [ "$actual" != "$expected" ]; then
    ng "$name" "expected: $expected" "actual:   $actual"
    return
  fi
  ok "$name"
}

# check_exit_code <名前> <期待コード> <sdd-state.sh の引数...>
check_exit_code() {
  local name="$1" want="$2"
  shift 2
  local actual code
  actual="$("$STATE" "$@" 2>/dev/null)"
  code=$?
  if [ "$code" -ne "$want" ]; then
    ng "$name" "終了コード: $code（期待: $want）" "stdout:   $(norm "$actual")"
    return
  fi
  ok "$name"
}

# ---------------------------------------------------------------------------
# (1) 各フィクスチャの自動選択 と (2) --feature での明示
# ---------------------------------------------------------------------------
for fixture in "$FIXTURES"/*/; do
  fixture="${fixture%/}"
  name="$(basename "$fixture")"
  [ -f "$fixture/expected.json" ] || continue

  check_state "state $name" "$fixture/expected.json" --root "$fixture"

  if [ -f "$fixture/feature.txt" ]; then
    feature="$(norm "$(cat "$fixture/feature.txt")")"
    check_state "state $name --feature $feature" \
      "$fixture/expected-feature.json" --root "$fixture" --feature "$feature"
  fi
done

# ---------------------------------------------------------------------------
# (3) 決定性: 同じ入力で 2 回実行して同じ出力
# ---------------------------------------------------------------------------
first="$(norm "$("$STATE" --root "$FIXTURES/03-implement-mid" 2>/dev/null)")"
second="$(norm "$("$STATE" --root "$FIXTURES/03-implement-mid" 2>/dev/null)")"
if [ -n "$first" ] && [ "$first" = "$second" ]; then
  ok "決定性 03-implement-mid"
else
  ng "決定性 03-implement-mid" "1 回目: $first" "2 回目: $second"
fi

# ---------------------------------------------------------------------------
# (4) 異常系: 終了コード 2
# ---------------------------------------------------------------------------
check_exit_code "異常系 --root が存在しない" 2 --root "$FIXTURES/99-does-not-exist"
check_exit_code "異常系 --feature が存在しない" 2 \
  --root "$FIXTURES/01-before-plan" --feature "specs/999-x"
check_exit_code "異常系 未知の引数" 2 --root "$FIXTURES/01-before-plan" --bogus

# ---------------------------------------------------------------------------
# (5) ガード: gh が使えないとき gh-unavailable を返し、state を素通しする
# ---------------------------------------------------------------------------
fake_bin="$tmpdir/bin"
mkdir -p "$fake_bin"
cat > "$fake_bin/gh" <<'FAKE_GH'
#!/usr/bin/env bash
exit 1
FAKE_GH
chmod +x "$fake_bin/gh"

state_json="$(norm "$(cat "$FIXTURES/01-before-plan/expected.json")")"
guard_expected="{\"go\":false,\"reason\":\"gh-unavailable\",\"state\":$state_json}"
guard_actual="$(printf '%s\n' "$state_json" \
  | PATH="$fake_bin:$PATH" "$GUARD" --repo example/repo --root "$FIXTURES/01-before-plan" 2>/dev/null)"
guard_code=$?
guard_actual="$(norm "$guard_actual")"

if [ "$guard_code" -ne 0 ]; then
  ng "ガード gh-unavailable" "終了コード: $guard_code（期待: 0）" "actual:   $guard_actual"
elif [ "$guard_actual" != "$guard_expected" ]; then
  ng "ガード gh-unavailable" "expected: $guard_expected" "actual:   $guard_actual"
else
  ok "ガード gh-unavailable"
fi

cat > "$fake_bin/gh" <<'FAKE_GH_API_FAIL'
#!/usr/bin/env bash
if [ "$1" = "api" ] && [ "$2" = "/rate_limit" ]; then
  printf '{}\n'
  exit 0
fi
exit 1
FAKE_GH_API_FAIL
chmod +x "$fake_bin/gh"

guard_actual="$(printf '%s\n' "$state_json" \
  | PATH="$fake_bin:$PATH" "$GUARD" --repo example/repo --root "$FIXTURES/01-before-plan" 2>/dev/null)"
guard_code=$?
guard_actual="$(norm "$guard_actual")"

if [ "$guard_code" -ne 0 ]; then
  ng "ガード gh api 失敗" "終了コード: $guard_code（期待: 0）" "actual:   $guard_actual"
elif [ "$guard_actual" != "$guard_expected" ]; then
  ng "ガード gh api 失敗" "expected: $guard_expected" "actual:   $guard_actual"
else
  ok "ガード gh api 失敗"
fi

# ---------------------------------------------------------------------------
# (6) UI 分類: Phase 領域による実装ループの事前判定
# ---------------------------------------------------------------------------
ui_tmp="$tmpdir/ui"
mkdir -p "$ui_tmp/specs/010-ui"
printf '# Tasks\n\n## Phase 1: UI\n<!-- sdd-domains: frontend-ui, backend -->\n- [ ] T001\n\n## Phase 2: API\n<!-- sdd-domains: backend -->\n- [ ] T002\n' > "$ui_tmp/specs/010-ui/tasks.md"
ui_actual="$("$UI_CLASSIFY" --root "$ui_tmp" --feature specs/010-ui --phase 1 2>/dev/null)"
ui_actual="$(norm "$ui_actual")"
if [ "$ui_actual" = '{"ui_change":true,"source":"phase-domains","domains":["frontend-ui","backend"]}' ]; then
  ok "UI 分類 Phase frontend-ui"
else
  ng "UI 分類 Phase frontend-ui" "actual:   $ui_actual"
fi

ui_actual="$("$UI_CLASSIFY" --root "$ui_tmp" --feature specs/010-ui --phase 2 2>/dev/null)"
ui_actual="$(norm "$ui_actual")"
if [ "$ui_actual" = '{"ui_change":false,"source":"phase-domains","domains":["backend"]}' ]; then
  ok "UI 分類 Phase backend"
else
  ng "UI 分類 Phase backend" "actual:   $ui_actual"
fi

printf '# Tasks\n\n## Phase 1: Missing\n- [ ] T001\n' > "$ui_tmp/specs/010-ui/tasks.md"
"$UI_CLASSIFY" --root "$ui_tmp" --feature specs/010-ui --phase 1 >/dev/null 2>&1
ui_code=$?
if [ "$ui_code" -eq 3 ]; then
  ok "UI 分類 missing domains"
else
  ng "UI 分類 missing domains" "終了コード: $ui_code（期待: 3）"
fi

printf '# Tasks\n\n## Phase 1: Invalid\n<!-- sdd-domains: mobile -->\n- [ ] T001\n' > "$ui_tmp/specs/010-ui/tasks.md"
"$UI_CLASSIFY" --root "$ui_tmp" --feature specs/010-ui --phase 1 >/dev/null 2>&1
ui_code=$?
if [ "$ui_code" -eq 3 ]; then
  ok "UI 分類 unknown domain"
else
  ng "UI 分類 unknown domain" "終了コード: $ui_code（期待: 3）"
fi

printf '# Tasks\n\n## Phase 1: Duplicate\n<!-- sdd-domains: backend -->\n<!-- sdd-domains: documentation -->\n- [ ] T001\n' > "$ui_tmp/specs/010-ui/tasks.md"
"$UI_CLASSIFY" --root "$ui_tmp" --feature specs/010-ui --phase 1 >/dev/null 2>&1
ui_code=$?
if [ "$ui_code" -eq 3 ]; then
  ok "UI 分類 duplicate marker"
else
  ng "UI 分類 duplicate marker" "終了コード: $ui_code（期待: 3）"
fi

printf '# Tasks\n\n## Phase 1: Duplicate domain\n<!-- sdd-domains: backend, backend -->\n- [ ] T001\n' > "$ui_tmp/specs/010-ui/tasks.md"
"$UI_CLASSIFY" --root "$ui_tmp" --feature specs/010-ui --phase 1 >/dev/null 2>&1
ui_code=$?
if [ "$ui_code" -eq 3 ]; then
  ok "UI 分類 duplicate domain"
else
  ng "UI 分類 duplicate domain" "終了コード: $ui_code（期待: 3）"
fi

printf '# Tasks\n\n## Phase 1: Trailing comma\n<!-- sdd-domains: backend, -->\n- [ ] T001\n' > "$ui_tmp/specs/010-ui/tasks.md"
"$UI_CLASSIFY" --root "$ui_tmp" --feature specs/010-ui --phase 1 >/dev/null 2>&1
ui_code=$?
if [ "$ui_code" -eq 3 ]; then
  ok "UI 分類 trailing empty domain"
else
  ng "UI 分類 trailing empty domain" "終了コード: $ui_code（期待: 3）"
fi

# ---------------------------------------------------------------------------
# (7) ガード: --github-dir で PR 一覧をファイルから受け取り、マージ済みは git 履歴で決める
#     jq と git が無ければ失敗にする。
# ---------------------------------------------------------------------------
if command -v jq >/dev/null 2>&1 && command -v git >/dev/null 2>&1; then
  # 利用者の git 設定を読まない（署名やフックが混ざらないように）。
  tgit() {
    GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
      git -c user.name=sdd-test -c user.email=sdd-test@example.invalid \
          -c core.autocrlf=false -c commit.gpgsign=false -c init.defaultBranch=main "$@"
  }

  # make_repo <フィクスチャ名> <マージ済み PR "番号:head.ref:触るディレクトリ" ...>
  # フィクスチャを写した git リポジトリを作り、指定の PR を古い順にマージコミットとして積む。
  # 作ったリポジトリのパスを stdout に返す。
  make_repo() {
    local fixture="$1"
    shift
    local repo="$tmpdir/repo-$fixture-$RANDOM"
    mkdir -p "$repo"
    cp -R "$FIXTURES/$fixture/." "$repo/"
    tgit -C "$repo" init -q
    tgit -C "$repo" add -A
    tgit -C "$repo" commit -q -m "init" >/dev/null
    local spec num ref dir
    for spec in "$@"; do
      num="${spec%%:*}"
      ref="${spec#*:}"; ref="${ref%%:*}"
      dir="${spec##*:}"
      tgit -C "$repo" switch -q -c "$ref"
      printf 'pr %s\n' "$num" > "$repo/$dir/.pr-$num"
      tgit -C "$repo" add -A
      tgit -C "$repo" commit -q -m "pr $num" >/dev/null
      tgit -C "$repo" switch -q main
      tgit -C "$repo" merge -q --no-ff -m "Merge pull request #$num from example/$ref" "$ref" >/dev/null
      tgit -C "$repo" branch -q -D "$ref"
    done
    printf '%s' "$repo"
  }

  # pulls_json <ラベル or "-"> <"番号:head.ref" ...>  →  feature branch 向け PR の JSON
  pulls_json() {
    local label="$1"
    shift
    local out="[" sep="" spec num ref labels
    for spec in "$@"; do
      num="${spec%%:*}"
      ref="${spec#*:}"
      if [ "$label" = "-" ]; then labels="[]"; else labels="[{\"name\":\"$label\"}]"; fi
      feature="$(printf '%s' "$ref" | sed -n 's#^claude/sdd-\([0-9][0-9][0-9]\)-.*#\1#p')"
      out="$out$sep{\"number\":$num,\"head\":{\"ref\":\"$ref\"},\"base\":{\"ref\":\"claude/sdd-$feature-feature\"},\"labels\":$labels}"
      sep=","
    done
    printf '%s]' "$out"
  }

  # check_guard <名前> <期待 JSON> <リポジトリ> <closed JSON> <open JSON>
  check_guard() {
    local name="$1" expected="$2" repo="$3" closed="$4" open="$5"
    local gh_dir="$tmpdir/github-$RANDOM"
    mkdir -p "$gh_dir"
    printf '%s' "$closed" > "$gh_dir/pulls-closed.json"
    printf '%s' "$open" > "$gh_dir/pulls-open.json"
    local state actual code
    state="$(norm "$("$STATE" --root "$repo" 2>/dev/null)")"
    actual="$(printf '%s\n' "$state" \
      | PATH="$fake_bin:$PATH" "$GUARD" --root "$repo" --github-dir "$gh_dir" 2>"$tmpdir/guard.err")"
    code=$?
    actual="$(norm "$actual")"
    if [ "$code" -ne 0 ]; then
      ng "$name" "終了コード: $code（期待: 0）" "stderr: $(cat "$tmpdir/guard.err")" "actual:   $actual"
    elif [ "$actual" != "$expected" ]; then
      ng "$name" "expected: $expected" "actual:   $actual"
    else
      ok "$name"
    fi
  }

  st02="$(norm "$(cat "$FIXTURES/02-before-tasks/expected.json")")"
  st03="$(norm "$(cat "$FIXTURES/03-implement-mid/expected.json")")"
  st06="$(norm "$(cat "$FIXTURES/06-multi-feature/expected-feature.json")")"

  # plan がマージ済みで、次は tasks。ホップは 1
  repo="$(make_repo 02-before-tasks "1:claude/sdd-010-plan:specs/010-a")"
  check_guard "ガード github-dir go" \
    "{\"go\":true,\"state\":$st02,\"hops\":1,\"phase_retries\":0,\"open_prs\":[]}" \
    "$repo" "$(pulls_json sdd 1:claude/sdd-010-plan)" "[]"

  # 自動 PR が open なら待つ
  check_guard "ガード github-dir open-pr" \
    "{\"go\":false,\"reason\":\"open-pr\",\"state\":$st02,\"hops\":1,\"phase_retries\":0,\"open_prs\":[\"claude/sdd-010-tasks\"]}" \
    "$repo" "$(pulls_json sdd 1:claude/sdd-010-plan)" "$(pulls_json sdd 2:claude/sdd-010-tasks)"

  check_guard "ガード github-dir ラベル無し open PR も塞ぐ" \
    "{\"go\":false,\"reason\":\"open-pr\",\"state\":$st02,\"hops\":1,\"phase_retries\":0,\"open_prs\":[\"claude/sdd-010-tasks\"]}" \
    "$repo" "$(pulls_json sdd 1:claude/sdd-010-plan)" "$(pulls_json - 2:claude/sdd-010-tasks)"

  check_guard "ガード github-dir base.ref 欠落は fail-closed" \
    "{\"go\":false,\"reason\":\"gh-unavailable\",\"state\":$st02}" \
    "$repo" "$(pulls_json sdd 1:claude/sdd-010-plan)" \
    '[{"number":2,"head":{"ref":"claude/sdd-010-tasks"},"labels":[{"name":"sdd"}]}]'

  # main 向けの最終 PR は段階 PR の冪等ガードに含めない
  check_guard "ガード github-dir 最終 PR は段階 PR を塞がない" \
    "{\"go\":true,\"state\":$st02,\"hops\":1,\"phase_retries\":0,\"open_prs\":[]}" \
    "$repo" "$(pulls_json sdd 1:claude/sdd-010-plan)" \
    '[{"number":9,"head":{"ref":"claude/sdd-010-feature"},"base":{"ref":"main"},"labels":[{"name":"sdd"}]}]'

  # closed でも履歴に無い（マージされていない）PR や、sdd ラベルの無い PR は数えない
  repo="$(make_repo 02-before-tasks "1:claude/sdd-010-plan:specs/010-a")"
  check_guard "ガード github-dir 未マージ・ラベル無しは数えない" \
    "{\"go\":true,\"state\":$st02,\"hops\":1,\"phase_retries\":0,\"open_prs\":[]}" \
    "$repo" \
    '[{"number":7,"head":{"ref":"claude/sdd-010-tasks"},"labels":[{"name":"sdd"}]},
      {"number":2,"head":{"ref":"claude/sdd-010-tasks"},"labels":[]},
      {"number":1,"head":{"ref":"claude/sdd-010-plan"},"labels":[{"name":"sdd"}]}]' \
    "[]"

  # 組み込み GitHub ツールの形（labels が文字列の配列）でも sdd PR と分かる
  repo="$(make_repo 02-before-tasks "1:claude/sdd-010-plan:specs/010-a")"
  check_guard "ガード github-dir labels が文字列の配列" \
    "{\"go\":true,\"state\":$st02,\"hops\":1,\"phase_retries\":0,\"open_prs\":[]}" \
    "$repo" '[{"number":1,"head":{"ref":"claude/sdd-010-plan"},"labels":["sdd"]}]' "[]"

  # 同じフェーズのマージが 2 回に達したら止まる
  repo="$(make_repo 03-implement-mid \
    "1:claude/sdd-010-plan:specs/010-a" "2:claude/sdd-010-tasks:specs/010-a" \
    "3:claude/sdd-010-implement-p2:specs/010-a" "4:claude/sdd-010-implement-p2:specs/010-a")"
  check_guard "ガード github-dir phase-retry-limit" \
    "{\"go\":false,\"reason\":\"phase-retry-limit\",\"state\":$st03,\"hops\":4,\"phase_retries\":2,\"open_prs\":[]}" \
    "$repo" \
    "$(pulls_json sdd 4:claude/sdd-010-implement-p2 3:claude/sdd-010-implement-p2 2:claude/sdd-010-tasks 1:claude/sdd-010-plan)" \
    "[]"

  # 直近のマージ済み sdd PR が触った機能を対象にする（自動選択の 011 ではなく 010 → done）
  repo="$(make_repo 06-multi-feature "1:claude/sdd-010-implement-p1:specs/010-a")"
  check_guard "ガード github-dir 対象機能の確定" \
    "{\"go\":false,\"reason\":\"nothing-to-do\",\"state\":$st06}" \
    "$repo" "$(pulls_json sdd 1:claude/sdd-010-implement-p1)" "[]"

  # ファイルが無ければ呼び出し側の誤り（終了コード 2）
  actual="$(printf '%s\n' "$st02" | "$GUARD" --root "$repo" --github-dir "$tmpdir/no-such-dir" 2>/dev/null)"
  code=$?
  if [ "$code" -eq 2 ]; then
    ok "ガード github-dir ファイル無しは終了コード 2"
  else
    ng "ガード github-dir ファイル無しは終了コード 2" "終了コード: $code" "stdout: $(norm "$actual")"
  fi
else
  ng "ガード github-dir" "jq and git are required; run mise install"
fi

# ---------------------------------------------------------------------------
printf '\n%s\n' "PASS: $pass_count / FAIL: $fail_count"
[ "$fail_count" -eq 0 ] || exit 1
