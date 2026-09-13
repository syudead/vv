#!/bin/bash
# Claude Code のステータスライン用コマンド。表示は何も出さない。
#
# 目的は表示ではなく**受け取ることそのもの**である。使用率
# （`rate_limits.five_hour.used_percentage` / `rate_limits.seven_day.used_percentage`）は、
# Claude Code がステータスラインスクリプトへの stdin JSON にだけ渡している
# （research.md R-004）。フックの入力にはこの項目が無く、使用量に関するフックイベントも
# 無い。そこで受け取った JSON をそのままファイルに落とし、`/sdd-next` の手順 0.5
# （使用量ゲート）から読めるようにする。
#
# これは R-004 の「案 a」の配線であり、cloud セッションでステータスラインが実行されるか
# どうかは quickstart S3 のプローブで確かめる。実行されなければこのファイルは作られず、
# 手順 0.5 は「取得不能」としてゲートを飛ばす（FR-020 後段）。
#
# 手元の CLI では何もしない。保守者の開発機に副作用を持たせないためである。
set -uo pipefail

if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  cat >/dev/null
  exit 0
fi

out="${TMPDIR:-/tmp}/sdd-rate-limits.json"
cat > "$out.tmp" 2>/dev/null || exit 0
# 書き込み途中の中身を読ませないよう、書き終えてから置き換える。
mv -f "$out.tmp" "$out" 2>/dev/null || rm -f "$out.tmp"

# ステータスラインには何も出さない（stdout が表示内容になるため）。
exit 0
