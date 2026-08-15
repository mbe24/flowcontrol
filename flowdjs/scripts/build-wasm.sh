#!/usr/bin/env sh
# Build the flowcore wasm + wasm-bindgen (nodejs) glue into ../flowwasm/pkg — the
# artifact flowd.js loads at runtime. Runs inside the `flowd` Docker container
# because the native toolchain is sandboxed on the dev host. `pkg/` is gitignored;
# this regenerates it.
set -eu

WASM_BINDGEN_VERSION=0.2.127

docker compose run --rm flowd sh -c "
  set -eu
  cd /work
  rustup target add wasm32-unknown-unknown
  cargo build -p flowwasm --target wasm32-unknown-unknown --release
  cargo install wasm-bindgen-cli --version ${WASM_BINDGEN_VERSION} --quiet
  rm -rf /work/flowwasm/pkg
  wasm-bindgen --target nodejs --out-dir /work/flowwasm/pkg \
    /tmp/target/wasm32-unknown-unknown/release/flowwasm.wasm
"
echo "built flowwasm/pkg (loaded by flowd.js)"
