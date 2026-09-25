#!/bin/bash
# Codespaces の作成時に1回だけ動く。vv の起動に要るものを入れる。
# Go・Node・task の版は mise.toml が単一の出どころなので、mise で入れる。
set -euo pipefail

cd "$(dirname "$0")/.."

sudo apt-get update
sudo apt-get install -y --no-install-recommends ffmpeg

curl -fsSL https://mise.run | sh
export PATH="$HOME/.local/bin:$PATH"
echo 'eval "$(~/.local/bin/mise activate bash)"' >>~/.bashrc

mise trust
mise install
# task preview に要るのは Go の依存と web の npm 依存だけである。
# golangci-lint などの検査ツールまで入れる task setup は、作成を待たせるので呼ばない。
mise exec -- go mod download
mise exec -- npm --prefix web ci
