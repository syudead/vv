#!/bin/bash
# Claude Code の SessionStart フック。
#
# Claude Code on the web のコンテナはセッションごとに新品なので、依存も開発ツールも
# 入っていない。ここで先に取得しておかないと、最初の make lint / make test で
# golangci-lint のビルド（数分）や npm install を待つことになる。
# フック完了後のコンテナ状態はキャッシュされるため、ここで払った時間は次のセッションに
# 引き継がれる。
#
# 手元の CLI では何もしない（開発機の環境は各自のものなので触らない）。
set -euo pipefail

if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

cd "${CLAUDE_PROJECT_DIR:-$(dirname "$0")/../..}"

# 版の指定は Makefile に一本化してある。ここでは目標を呼ぶだけにする。
make setup

# ffmpeg／ffprobe はここでは入れない。make test と make lint には不要で、
# 必要になるのは bin/mdm を直接起動するときだけである（起動前確認で存在を見る）。
# 必要なら: apt-get update && apt-get install -y --no-install-recommends ffmpeg
