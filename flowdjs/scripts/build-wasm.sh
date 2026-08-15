#!/usr/bin/env sh
# The wasm glue is now generated for both JS hosts at once by the repo-root script.
# This shim is kept so existing references keep working.
exec sh "$(dirname "$0")/../../scripts/gen-wasm.sh"
