#!/bin/bash
# Warm dependencies and tools in fresh Claude Code web containers; the completed container is cached.
# Leave local CLI environments unchanged.
set -euo pipefail

if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

cd "${CLAUDE_PROJECT_DIR:-$(dirname "$0")/../..}"

# Read the task version from mise.toml, shared with CI.
task_version="$(sed -n 's/^task *= *"\(.*\)"$/\1/p' mise.toml)"
if [ -z "$task_version" ]; then
  echo "session-start: mise.toml に task の版が見つからない" >&2
  exit 1
fi
gobin="$(go env GOPATH)/bin"
# Reuse the cached installation when its version matches.
if ! "$gobin/task" --version 2>/dev/null | grep -q "v\?${task_version}\b"; then
  go install "github.com/go-task/task/v3/cmd/task@v${task_version}"
fi

# Run npm ci only when lockfiles change; task setup would otherwise replace cached node_modules on every start.
npm_ci_if_changed() {
  local dir="$1" stamp="$1/node_modules/.session-start-lock.sha256" sum
  sum="$(sha256sum "$dir/package-lock.json" | cut -d' ' -f1)"
  if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$sum" ]; then
    return 0
  fi
  npm --prefix "$dir" ci
  echo "$sum" >"$stamp"
}
go mod download
go -C tools mod download
for dir in web tools docs-site; do npm_ci_if_changed "$dir"; done
"$(go -C tools tool -n golangci-lint)" --version
go -C tools tool air -v
go -C tools tool oapi-codegen -version
go build ./...

if [ -n "${CLAUDE_ENV_FILE:-}" ]; then
  echo "export PATH=\"$gobin:\$PATH\"" >>"$CLAUDE_ENV_FILE"
fi

# FFmpeg supports startup checks, media tests and e2e fixtures. Installation failure must not block lint or unit tests.
has_media_tools() { command -v ffmpeg >/dev/null && command -v ffprobe >/dev/null; }
if ! has_media_tools; then
  sudo=""
  if [ "$(id -u)" != "0" ]; then sudo="sudo"; fi
  # Pass DEBIAN_FRONTEND inside sudo because sudo drops environment variables.
  apt_install() {
    $sudo env DEBIAN_FRONTEND=noninteractive \
      apt-get install -y -qq --no-install-recommends "$@" ffmpeg
  }
  # Blocked PPAs may warn without failing apt update.
  $sudo apt-get update -qq || true
  apt_install || true
  # Reinstall when the package exists but its binaries are missing.
  if ! has_media_tools; then apt_install --reinstall || true; fi
  if ! has_media_tools; then
    echo "session-start: ffmpeg／ffprobe を入れられなかった（メディア系のテストと e2e は動かない）" >&2
  fi
fi

# Advertise the Workers AI relay when Ollama is unavailable and an account ID is configured.
if [ -n "${CLOUDFLARE_ACCOUNT_ID:-}" ]; then
  echo "session-start: 自動タグ付けは Ollama の代わりに go run ./scripts/clefrelay（Workers AI の Clef への中継）で試せる。使い方は scripts/clefrelay/main.go の冒頭。"
fi
