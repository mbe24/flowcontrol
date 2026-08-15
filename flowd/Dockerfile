# Dev/test image for flowd. Builds and runs in a Linux container so a host whose
# policy blocks native build scripts (Windows sandbox: "Access is denied" on
# build-script-build) can still compile and test the crate. Cache volumes +
# CARGO_TARGET_DIR redirection keep rebuilds fast (see docker-compose.yml). Run
# tests/build via root npm scripts: `npm run test:flowd` / `build:flowd` /
# `check:flowd` (each wraps `docker compose run --rm flowd <cargo …>`).
#
# Layer strategy (few, explicitly ordered so the cheap layer is cached longest):
#   1. WORKDIR                     - trivial, earliest so it need not re-run
#   2. apt deps                    - changes rarely; its own layer
#   3. cargo-watch                 - the expensive compile; last. A BuildKit
#                                    cache mount keeps re-downloads incremental
#                                    if this layer ever does rebuild.
#
# No flowd binary is baked in: it compiles at runtime via cargo-watch so editing
# server code live-reloads instead of requiring an image rebuild.
ARG RUST_IMAGE=rust:1-slim-bookworm
FROM $RUST_IMAGE
WORKDIR /work/flowd

RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config libssl-dev \
    && rm -rf /var/lib/apt/lists/*

# cargo-watch recompiles and restarts `flowd serve` when a Rust file changes.
# The registry cache keeps downloaded crates out of the image layers and makes
# repeated `cargo install` fast; the compiled binary lands in the image.
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    cargo install cargo-watch --locked
