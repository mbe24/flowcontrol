import { defineConfig } from "tsup";

// flowmcp is a stdio CLI (the `flowmcp` bin), not a library — no dts. All deps stay
// external (installed via package.json): flowdjs (for ensureDaemon), @flow/api,
// @connectrpc/*, @bufbuild/protobuf, @modelcontextprotocol/*, zod. Only our own src
// is bundled. flowmcp never loads the wasm itself — it connects to / spawns the daemon.
export default defineConfig({
  entry: ["src/index.ts"],
  format: ["esm"],
  platform: "node",
  target: "node22",
  dts: false,
  sourcemap: true,
  clean: true,
  banner: { js: "#!/usr/bin/env node" }
});
