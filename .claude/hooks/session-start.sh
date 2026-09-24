#!/bin/bash
# Claude Code の SessionStart フック。
#
# Claude Code on the web のコンテナはセッションごとに新品なので、依存も開発ツールも
# 入っていない。ここで先に取得しておかないと、最初の task lint / task test で
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

# task の版は mise.toml（CI の mise-action と同じ出どころ）の [tools] から読む。
task_version="$(sed -n 's/^task *= *"\(.*\)"$/\1/p' mise.toml)"
if [ -z "$task_version" ]; then
  echo "session-start: mise.toml に task の版が見つからない" >&2
  exit 1
fi
go install "github.com/go-task/task/v3/cmd/task@v${task_version}"
gobin="$(go env GOPATH)/bin"
"$gobin/task" setup

# 以後のコマンドから task をそのまま呼べるように、セッションの PATH に足す。
if [ -n "${CLAUDE_ENV_FILE:-}" ]; then
  echo "export PATH=\"$gobin:\$PATH\"" >>"$CLAUDE_ENV_FILE"
fi

# ffmpeg／ffprobe はここでは入れない。task test と task lint には不要で、
# 必要になるのは bin/mdm を直接起動するときだけである（起動前確認で存在を見る）。
# 必要なら: apt-get update && apt-get install -y --no-install-recommends ffmpeg
