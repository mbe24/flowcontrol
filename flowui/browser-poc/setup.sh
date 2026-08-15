#!/usr/bin/env sh
# Populate the two vendored/generated asset dirs this POC needs (both gitignored):
#   flowwasm/  — the browser (--target web) wasm-bindgen glue for flowwasm
#   sqlite3/   — @sqlite.org/sqlite-wasm's init module + wasm
# Run from this directory. Requires the web glue to have been built first
# (../../flowdjs/scripts/build-wasm.sh builds the nodejs glue; see below for web).
set -eu
cd "$(dirname "$0")"
ROOT=../..

mkdir -p flowwasm sqlite3

# The web glue: build it the same way as the nodejs glue but `--target web`.
# (One-liner via the flowd container — mirrors flowdjs/scripts/build-wasm.sh.)
if [ ! -f "$ROOT/flowwasm/pkg-web/flowwasm.js" ]; then
  echo "building browser wasm glue into flowwasm/pkg-web ..."
  ( cd "$ROOT" && docker compose run --rm flowd sh -c '
      set -eu; cd /work
      rustup target add wasm32-unknown-unknown
      cargo build -p flowwasm --target wasm32-unknown-unknown --release
      cargo install wasm-bindgen-cli --version 0.2.127 --quiet
      rm -rf /work/flowwasm/pkg-web
      wasm-bindgen --target web --out-dir /work/flowwasm/pkg-web \
        /tmp/target/wasm32-unknown-unknown/release/flowwasm.wasm
  ' )
fi
cp "$ROOT/flowwasm/pkg-web/flowwasm.js" "$ROOT/flowwasm/pkg-web/flowwasm_bg.wasm" flowwasm/

# sqlite-wasm assets (installed as a flowui dependency).
SQ="$ROOT/node_modules/@sqlite.org/sqlite-wasm/dist"
cp "$SQ/index.mjs" "$SQ/sqlite3.wasm" sqlite3/

echo "ready. run:  node server.mjs . 8099   then open http://127.0.0.1:8099/"
