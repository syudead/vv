#!/usr/bin/env bash
# SDD ハーネスの判定テスト。
#
# 依存は bash・coreutils（mktemp・tr・cat）だけである。`jq` も `gh` も要らない
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

# ---------------------------------------------------------------------------
printf '\n%s\n' "PASS: $pass_count / FAIL: $fail_count"
[ "$fail_count" -eq 0 ] || exit 1
