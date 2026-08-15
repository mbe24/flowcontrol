import { defineConfig } from "tsup";

// Two entries: the daemon (index) and the library ensureDaemon/session exports
// (lib, imported by flowmcp). All dependencies — including @flow/api — stay
// external (installed via package.json); only our own src is bundled. The wasm
// glue is NOT a bundle input: store.ts loads it at runtime via createRequire from
// ./wasm, which scripts/bundle-wasm.mjs copies in after the build.
export default defineConfig({
  entry: ["src/index.ts", "src/lib.ts"],
  format: ["esm"],
  platform: "node",
  target: "node22",
  dts: { entry: "src/lib.ts" },
  sourcemap: true,
  clean: true,
  // index.js is the `flowd-js` bin; a shebang makes it directly executable. node
  // strips a leading shebang when the file is imported/spawned, so it's harmless on lib.js.
  banner: { js: "#!/usr/bin/env node" }
});
