import { defineConfig } from "tsup";

// flowmcp is a stdio CLI (the `flowmcp` bin), not a library — no dts. @flow/api is
// bundled IN (noExternal) — generated protobuf glue we keep internal, never published.
// All other deps stay external (installed via package.json): flowdjs (for ensureDaemon),
// @connectrpc/*, @bufbuild/protobuf, @modelcontextprotocol/*, zod. flowmcp never loads
// the wasm itself — it connects to / spawns the daemon.
export default defineConfig({
  entry: ["src/index.ts"],
  format: ["esm"],
  platform: "node",
  target: "node22",
  noExternal: [/^@flow\/api(\/|$)/],
  dts: false,
  sourcemap: true,
  clean: true,
  banner: { js: "#!/usr/bin/env node" }
});
