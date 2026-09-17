#!/usr/bin/env bash
# SDD ハーネスの判定テスト。
#
# 依存は bash・coreutils（mktemp・tr・cat）・jq・git。`gh` も GitHub API も要らない
# （research.md R-007 / US3: 保守者の手元 = Windows の Git Bash で検算できること）。
#
# 検査するもの:
#   (1) fixtures/*/  … sdd-state.sh --root <fixture> の出力が expected.json と一致
#   (2) feature.txt を持つフィクスチャ … --feature でも expected-feature.json と一致
#   (3) 決定性     … 同じ入力で 2 回実行して同じ出力
#   (4) 異常系     … 引数の誤りで終了コード 2
#   (5) workflow   … UI Issue だけ design 段階を挟む
#   (6) ガード     … git だけで判定し、remote が無ければ remote-unavailable で state を素通しする
#
# 出力は PASS/FAIL の 1 行ずつ。失敗が 1 つでもあれば終了コード 1。

set -u

dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS="$dir/../scripts"
FIXTURES="$dir/fixtures"
STATE="$SCRIPTS/sdd-state.sh"
GUARD="$SCRIPTS/sdd-guard.sh"
TARGET="$SCRIPTS/sdd-target.sh"

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
# ---------------------------------------------------------------------------
# (5) workflow: UI Issue だけ design 段階を挟む
# ---------------------------------------------------------------------------
ui_actual="$("$STATE" --root "$FIXTURES/02-before-tasks" --feature specs/010-a --workflow ui 2>/dev/null)"
ui_actual="$(norm "$ui_actual")"
ui_expected='{"feature_dir":"specs/010-a","feature":"010","stage":"design","workflow":"ui","feature_branch":"claude/sdd-010-feature","base_branch":"claude/sdd-010-feature","branch":"claude/sdd-010-design"}'
if [ "$ui_actual" = "$ui_expected" ]; then
  ok "workflow ui design"
else
  ng "workflow ui design" "expected: $ui_expected" "actual:   $ui_actual"
fi

ui_tmp="$tmpdir/ui"
mkdir -p "$ui_tmp"
cp -R "$FIXTURES/02-before-tasks/." "$ui_tmp/"
printf '# UI design\n' > "$ui_tmp/specs/010-a/ui-design.md"
ui_actual="$("$STATE" --root "$ui_tmp" --feature specs/010-a --workflow ui 2>/dev/null)"
ui_actual="$(norm "$ui_actual")"
ui_expected='{"feature_dir":"specs/010-a","feature":"010","stage":"tasks","workflow":"ui","feature_branch":"claude/sdd-010-feature","base_branch":"claude/sdd-010-feature","branch":"claude/sdd-010-tasks"}'
if [ "$ui_actual" = "$ui_expected" ]; then
  ok "workflow ui tasks after design"
else
  ng "workflow ui tasks after design" "expected: $ui_expected" "actual:   $ui_actual"
fi

ui_actual="$("$STATE" --root "$FIXTURES/01-before-plan" --feature specs/010-a --workflow ui 2>/dev/null)"
ui_actual="$(norm "$ui_actual")"
ui_expected='{"feature_dir":"specs/010-a","feature":"010","stage":"plan","workflow":"ui","feature_branch":"claude/sdd-010-feature","base_branch":"claude/sdd-010-feature","branch":"claude/sdd-010-plan"}'
if [ "$ui_actual" = "$ui_expected" ]; then
  ok "workflow ui plan first"
else
  ng "workflow ui plan first" "expected: $ui_expected" "actual:   $ui_actual"
fi

"$STATE" --root "$FIXTURES/01-before-plan" --workflow other >/dev/null 2>&1
ui_code=$?
if [ "$ui_code" -eq 2 ]; then
  ok "workflow unknown"
else
  ng "workflow unknown" "終了コード: $ui_code（期待: 2）"
fi

# ---------------------------------------------------------------------------
# (6) ガード: git だけで判定する。remote が無ければ remote-unavailable で state を素通しする。
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
  # フィクスチャを写した git リポジトリと bare の origin を作る。PR が 1 件でもあれば
  # `claude/sdd-NNN-feature` を main から切り、指定の PR を古い順に merge commit として
  # feature branch に積み、main と feature を origin に push する。HEAD は feature branch
  # （PR が無ければ main）。作ったリポジトリのパスを stdout に返す。
  make_repo() {
    local fixture="$1"
    shift
    local repo="$tmpdir/repo-$fixture-$RANDOM"
    mkdir -p "$repo"
    cp -R "$FIXTURES/$fixture/." "$repo/"
    tgit -C "$repo" init -q
    tgit -C "$repo" add -A
    tgit -C "$repo" commit -q -m "init" >/dev/null
    tgit init -q --bare "$repo.git"
    tgit -C "$repo" remote add origin "$repo.git"
    tgit -C "$repo" push -q origin main
    local spec num ref dir feature_branch=""
    for spec in "$@"; do
      num="${spec%%:*}"
      ref="${spec#*:}"; ref="${ref%%:*}"
      dir="${spec##*:}"
      if [ -z "$feature_branch" ]; then
        feature_branch="$(printf '%s' "$ref" | sed -n 's#^\(claude/sdd-[0-9][0-9][0-9]-\).*#\1feature#p')"
        tgit -C "$repo" switch -q -c "$feature_branch"
      fi
      tgit -C "$repo" switch -q -c "$ref"
      printf 'pr %s\n' "$num" > "$repo/$dir/.pr-$num"
      tgit -C "$repo" add -A
      tgit -C "$repo" commit -q -m "pr $num" >/dev/null
      tgit -C "$repo" switch -q "$feature_branch"
      tgit -C "$repo" merge -q --no-ff -m "Merge pull request #$num from example/$ref" "$ref" >/dev/null
      tgit -C "$repo" branch -q -D "$ref"
    done
    [ -z "$feature_branch" ] || tgit -C "$repo" push -q origin "$feature_branch"
    printf '%s' "$repo"
  }

  # push_branch <リポジトリ> <branch 名>  … 進行中の段階 PR を模して remote に branch を置く
  push_branch() {
    tgit -C "$1" push -q origin "HEAD:refs/heads/$2"
  }

  # check_guard <名前> <期待 JSON> <リポジトリ> [workflow]
  check_guard() {
    local name="$1" expected="$2" repo="$3"
    local workflow="${4:-standard}"
    local state actual code
    state="$(norm "$("$STATE" --root "$repo" --workflow "$workflow" 2>/dev/null)")"
    actual="$(printf '%s\n' "$state" | "$GUARD" --root "$repo" 2>"$tmpdir/guard.err")"
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

  st01="$(norm "$(cat "$FIXTURES/01-before-plan/expected.json")")"
  st02="$(norm "$(cat "$FIXTURES/02-before-tasks/expected.json")")"
  st03="$(norm "$(cat "$FIXTURES/03-implement-mid/expected.json")")"

  # remote が無ければ fail-closed
  repo="$tmpdir/repo-no-remote"
  mkdir -p "$repo"
  cp -R "$FIXTURES/01-before-plan/." "$repo/"
  tgit -C "$repo" init -q
  tgit -C "$repo" add -A
  tgit -C "$repo" commit -q -m "init" >/dev/null
  check_guard "ガード remote-unavailable" \
    "{\"go\":false,\"reason\":\"remote-unavailable\",\"state\":$st01}" "$repo"

  # spec が main に入った直後（feature branch はまだ無い）。main 上で plan へ進む
  repo="$(make_repo 01-before-plan)"
  check_guard "ガード 初回は main から plan" \
    "{\"go\":true,\"state\":$st01,\"hops\":0,\"phase_retries\":0,\"open_prs\":[]}" "$repo"

  # plan がマージ済みで、次は tasks。ホップは 1
  repo="$(make_repo 02-before-tasks "1:claude/sdd-010-plan:specs/010-a")"
  check_guard "ガード go" \
    "{\"go\":true,\"state\":$st02,\"hops\":1,\"phase_retries\":0,\"open_prs\":[]}" "$repo"

  st02_ui="$(norm "$("$STATE" --root "$repo" --feature specs/010-a --workflow ui 2>/dev/null)")"
  check_guard "ガード UI workflow 維持" \
    "{\"go\":true,\"state\":$st02_ui,\"hops\":1,\"phase_retries\":0,\"open_prs\":[]}" "$repo" ui

  # feature branch が remote にあるのに main 上なら止める（手打ちの取り違えなど）
  tgit -C "$repo" switch -q main
  check_guard "ガード wrong-base" \
    "{\"go\":false,\"reason\":\"wrong-base\",\"state\":$(norm "$("$STATE" --root "$repo" 2>/dev/null)"),\"feature_branch\":\"claude/sdd-010-feature\",\"current_branch\":\"main\"}" \
    "$repo"
  tgit -C "$repo" switch -q claude/sdd-010-feature

  # 段階 branch が remote に残っていれば待つ（label の有無に依らない）
  push_branch "$repo" claude/sdd-010-tasks
  check_guard "ガード open-pr" \
    "{\"go\":false,\"reason\":\"open-pr\",\"state\":$st02,\"hops\":1,\"phase_retries\":0,\"open_prs\":[\"claude/sdd-010-tasks\"]}" \
    "$repo"
  tgit -C "$repo" push -q origin --delete claude/sdd-010-tasks

  # 他の機能の branch は数えない
  push_branch "$repo" claude/sdd-011-plan
  check_guard "ガード 他機能の branch は塞がない" \
    "{\"go\":true,\"state\":$st02,\"hops\":1,\"phase_retries\":0,\"open_prs\":[]}" "$repo"

  # squash マージ（件名 "(#N)"）は head 名が分からないので hop に数えない
  repo="$(make_repo 02-before-tasks "1:claude/sdd-010-plan:specs/010-a")"
  printf 'squash\n' > "$repo/specs/010-a/.pr-2"
  tgit -C "$repo" add -A
  tgit -C "$repo" commit -q -m "docs: 010 の実装タスクを分解する (#2)" >/dev/null
  check_guard "ガード squash は hop に数えない" \
    "{\"go\":true,\"state\":$st02,\"hops\":1,\"phase_retries\":0,\"open_prs\":[]}" "$repo"

  # 同じフェーズのマージが 2 回に達したら止まる
  repo="$(make_repo 03-implement-mid \
    "1:claude/sdd-010-plan:specs/010-a" "2:claude/sdd-010-tasks:specs/010-a" \
    "3:claude/sdd-010-implement-p2:specs/010-a" "4:claude/sdd-010-implement-p2:specs/010-a")"
  check_guard "ガード phase-retry-limit" \
    "{\"go\":false,\"reason\":\"phase-retry-limit\",\"state\":$st03,\"hops\":4,\"phase_retries\":2,\"open_prs\":[]}" \
    "$repo"

  # target は --pr で渡された PR が触った機能を選ぶ。guard は対象を差し替えない。
  repo="$(make_repo 06-multi-feature "1:claude/sdd-010-implement-p1:specs/010-a")"
  actual="$("$TARGET" --root "$repo" --pr 1)"
  if [ "$actual" = "specs/010-a" ]; then
    ok "target --pr 対象機能の確定"
  else
    ng "target --pr 対象機能の確定" "expected: specs/010-a" "actual: $actual"
  fi
  actual="$("$TARGET" --root "$repo" --pr 99)"
  if [ "$actual" = "specs/011-b" ]; then
    ok "target --pr 見つからなければ未完了の先頭"
  else
    ng "target --pr 見つからなければ未完了の先頭" "expected: specs/011-b" "actual: $actual"
  fi
  actual="$("$TARGET" --root "$repo")"
  if [ "$actual" = "specs/011-b" ]; then
    ok "target --pr 無しは未完了の先頭"
  else
    ng "target --pr 無しは未完了の先頭" "expected: specs/011-b" "actual: $actual"
  fi
  "$TARGET" --root "$repo" --pr abc >/dev/null 2>&1
  code=$?
  if [ "$code" -eq 2 ]; then
    ok "target --pr が数字でなければ終了コード 2"
  else
    ng "target --pr が数字でなければ終了コード 2" "終了コード: $code"
  fi
  check_guard "ガードは対象機能を差し替えない" \
    "{\"go\":true,\"state\":$(norm "$("$STATE" --root "$repo")"),\"hops\":0,\"phase_retries\":0,\"open_prs\":[]}" \
    "$repo"

  # 引数の誤りは終了コード 2
  actual="$(printf '%s\n' "$st02" | "$GUARD" --root "$repo" --github-dir /nowhere 2>/dev/null)"
  code=$?
  if [ "$code" -eq 2 ]; then
    ok "ガード 未知の引数は終了コード 2"
  else
    ng "ガード 未知の引数は終了コード 2" "終了コード: $code" "stdout: $(norm "$actual")"
  fi
else
  ng "ガード" "jq and git are required; run mise install"
fi

# ---------------------------------------------------------------------------
printf '\n%s\n' "PASS: $pass_count / FAIL: $fail_count"
[ "$fail_count" -eq 0 ] || exit 1
