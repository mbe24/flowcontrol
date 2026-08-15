#!/usr/bin/env sh
# Copy the browser (--target web) wasm-bindgen glue into src so Vite bundles it.
# Build the glue first: ../flowui/browser-poc/setup.sh (or flowdjs/scripts/build-wasm.sh
# with --target web). The dir is gitignored.
set -eu
cd "$(dirname "$0")/.."
SRC=../flowwasm/pkg-web
[ -f "$SRC/flowwasm.js" ] || { echo "missing $SRC — build the web glue first"; exit 1; }
mkdir -p src/lib/browser/wasm
cp "$SRC/flowwasm.js" "$SRC/flowwasm.d.ts" "$SRC/flowwasm_bg.wasm" "$SRC/flowwasm_bg.wasm.d.ts" src/lib/browser/wasm/
echo "copied web glue into src/lib/browser/wasm/"
