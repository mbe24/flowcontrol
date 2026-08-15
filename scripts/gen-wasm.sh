#!/usr/bin/env sh
# Generate the flowcore wasm + wasm-bindgen glue for BOTH JS hosts in one pass:
#   flowwasm/pkg      — nodejs target, loaded by flowd.js (flowdjs)
#   flowwasm/pkg-web  — web target,    bundled by flowui (via flowui/scripts/copy-wasm.sh)
#
# The native toolchain is sandboxed on the dev host, so this runs in the `flowd`
# Docker container. Both output dirs are gitignored; this is the single, reproducible
# regeneration step — CI runs it before building the npm packages and the Pages site.
#
# Usage:  npm run gen:wasm     (or: sh scripts/gen-wasm.sh)
set -eu

WASM_BINDGEN_VERSION=0.2.127

docker compose run --rm flowd sh -c "
  set -eu
  cd /work
  rustup target add wasm32-unknown-unknown
  cargo build -p flowwasm --target wasm32-unknown-unknown --release
  cargo install wasm-bindgen-cli --version ${WASM_BINDGEN_VERSION} --quiet
  rm -rf /work/flowwasm/pkg /work/flowwasm/pkg-web
  wasm-bindgen --target nodejs --out-dir /work/flowwasm/pkg \
    /tmp/target/wasm32-unknown-unknown/release/flowwasm.wasm
  wasm-bindgen --target web --out-dir /work/flowwasm/pkg-web \
    /tmp/target/wasm32-unknown-unknown/release/flowwasm.wasm
"
echo "generated flowwasm/pkg (nodejs, for flowd.js) + flowwasm/pkg-web (web, for flowui)"
