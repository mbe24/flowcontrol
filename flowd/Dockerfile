# Dev/test image for flowd. Builds and runs in a Linux container so a host whose
# policy blocks native build scripts (Windows sandbox: "Access is denied" on
# build-script-build) can still compile and test the crate. Cache volumes +
# CARGO_TARGET_DIR redirection keep rebuilds fast (see scripts/native-runner.mjs).
ARG RUST_IMAGE=rust:1-slim-bookworm
FROM $RUST_IMAGE
RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config libssl-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /work/flowd
