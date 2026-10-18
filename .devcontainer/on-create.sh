#!/bin/bash
# Install runtime requirements once when Codespaces is created; mise.toml pins tool versions.
set -euo pipefail

cd "$(dirname "$0")/.."

sudo apt-get update
sudo apt-get install -y --no-install-recommends ffmpeg

curl -fsSL https://mise.run | sh
export PATH="$HOME/.local/bin:$PATH"
echo 'eval "$(~/.local/bin/mise activate bash)"' >>~/.bashrc

mise trust
mise install
# Preview needs Go and web dependencies only; installing lint tools would delay creation.
mise exec -- go mod download
mise exec -- npm --prefix web ci
