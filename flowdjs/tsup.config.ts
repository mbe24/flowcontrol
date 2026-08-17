import { defineConfig } from "tsup";

// Two entries: the daemon (index) and the library ensureDaemon/session exports
// (lib, imported by flowmcp). @flow/api is bundled IN (noExternal) — it's generated
// protobuf glue we keep internal, never published, so it must ride inside dist.
// All other deps stay external (installed via package.json). The wasm glue is NOT a
// bundle input: store.ts loads it at runtime via createRequire from ./wasm, which
// scripts/bundle-wasm.mjs copies in after the build.
export default defineConfig({
  entry: ["src/index.ts", "src/lib.ts"],
  format: ["esm"],
  platform: "node",
  target: "node22",
  noExternal: [/^@flow\/api(\/|$)/],
  dts: { entry: "src/lib.ts" },
  sourcemap: true,
  clean: true,
  // index.js is the `flowd-js` bin; a shebang makes it directly executable. node
  // strips a leading shebang when the file is imported/spawned, so it's harmless on lib.js.
  banner: { js: "#!/usr/bin/env node" }
});
