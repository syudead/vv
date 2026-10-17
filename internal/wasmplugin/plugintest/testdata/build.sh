#!/bin/sh
# Regenerate with WASI SDK 25. Ordinary go test runs use the checked-in binaries.
set -eu
: "${WASI_SDK_PATH:?Set WASI_SDK_PATH to an extracted WASI SDK 25 directory}"
cd "$(dirname "$0")"
for fixture in probe cli; do
  "$WASI_SDK_PATH/bin/clang" --sysroot="$WASI_SDK_PATH/share/wasi-sysroot" \
    -O2 -Wall -Wextra -Werror -Wno-unused-function -Wl,--strip-all \
    "$fixture.c" -o "$fixture.wasm"
  chmod 644 "$fixture.wasm"
done
