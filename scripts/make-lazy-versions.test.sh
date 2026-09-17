#!/usr/bin/env bash
set -eu

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
mkdir -p "$tmpdir/bin"

printf '%s\n' \
  '#!/usr/bin/env sh' \
  ': > "$VV_JQ_MARKER"' \
  'exit 99' > "$tmpdir/bin/jq"
chmod +x "$tmpdir/bin/jq"

VV_JQ_MARKER="$tmpdir/jq-called" PATH="$tmpdir/bin:$PATH" \
  make --dry-run help up down dev >/dev/null

if [ -e "$tmpdir/jq-called" ]; then
  echo "jq was invoked by a Make target that must not require it" >&2
  exit 1
fi

echo "PASS help/up/down/dev do not invoke jq"
