// On stdio, stdout IS the wire — a stray console.log corrupts framing. Route all
// diagnostics to stderr (belt-and-braces; the spec also deprecates the MCP logging
// primitive in favor of stderr). Must run before anything might log.
console.log = console.error;

import { McpServer } from "@modelcontextprotocol/server";
import { serveStdio } from "@modelcontextprotocol/server/stdio";
import { createFlowClient, DEFAULT_FLOWD_ADDR } from "./flowd";
import { tools } from "./tools/all";
import { registerTools, type ToolDeps } from "./tools/registry";

const addr = process.env.FLOWD_ADDR ?? DEFAULT_FLOWD_ADDR;
const deps: ToolDeps = {
  flow: createFlowClient(addr),
  callTimeoutMs: 10_000,
  author: process.env.FLOWMCP_AUTHOR ?? "flowmcp",
};

// Degraded-mode startup: probe flowd but KEEP SERVING if it's down. Hosts launch
// stdio servers eagerly; exiting would leave the server dead until the host
// restarts. `server/discover` and `tools/list` work without flowd; each tool call
// returns a retryable error until it is up.
void (async () => {
  try {
    await deps.flow.listProjects({ includeArchived: false }, { timeoutMs: 3_000 });
    console.error(`[flowmcp] connected to flowd at ${addr}`);
  } catch {
    console.error(
      `[flowmcp] flowd unreachable at ${addr} — set FLOWD_ADDR or run ` +
        "`docker compose up flowd`. Serving anyway; tool calls error until it is up.",
    );
  }
})();

// serveStdio owns the 2026-07-28 vs legacy era negotiation and pins one instance
// per connection. The factory registers the (static) tool set each time.
serveStdio(() => {
  const server = new McpServer(
    { name: "flowmcp", version: "0.1.0" },
    { capabilities: { tools: {} } },
  );
  registerTools(server, deps, tools);
  return server;
});
