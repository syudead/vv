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

# ffmpeg／ffprobe は bin/mdm の起動、internal/media の *WithFFmpeg テスト、e2e の
# メディア fixture 生成（web/e2e/media-fixtures.mjs）で要る。無いとセッションごとに
# 足りないと気づいて止まるので、ここで入れておく（フック後の状態はキャッシュされる）。
# 取得に失敗しても lint や単体テストは進められるので、フック全体は失敗させない。
has_media_tools() { command -v ffmpeg >/dev/null && command -v ffprobe >/dev/null; }
if ! has_media_tools; then
  sudo=""
  if [ "$(id -u)" != "0" ]; then sudo="sudo"; fi
  # sudo は環境変数を捨てるので、DEBIAN_FRONTEND は env で sudo の内側に渡す。
  apt_install() {
    $sudo env DEBIAN_FRONTEND=noninteractive \
      apt-get install -y -qq --no-install-recommends "$@" ffmpeg
  }
  # 一部の PPA がプロキシで拒否されても update 自体は警告で済む。
  $sudo apt-get update -qq || true
  apt_install || true
  # パッケージは入っているのに実行ファイルだけ欠けている場合は再展開する。
  if ! has_media_tools; then apt_install --reinstall || true; fi
  if ! has_media_tools; then
    echo "session-start: ffmpeg／ffprobe を入れられなかった（メディア系のテストと e2e は動かない）" >&2
  fi
fi
