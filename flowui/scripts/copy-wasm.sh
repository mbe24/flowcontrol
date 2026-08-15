#!/usr/bin/env sh
# Copy the browser (--target web) wasm-bindgen glue into src so Vite bundles it.
# Generate it first with `npm run gen:wasm` (produces flowwasm/pkg-web). The dir
# copied into is gitignored.
set -eu
cd "$(dirname "$0")/.."
SRC=../flowwasm/pkg-web
[ -f "$SRC/flowwasm.js" ] || { echo "missing $SRC — run \`npm run gen:wasm\` first"; exit 1; }
mkdir -p src/lib/browser/wasm
cp "$SRC/flowwasm.js" "$SRC/flowwasm.d.ts" "$SRC/flowwasm_bg.wasm" "$SRC/flowwasm_bg.wasm.d.ts" src/lib/browser/wasm/
echo "copied web glue into src/lib/browser/wasm/"
