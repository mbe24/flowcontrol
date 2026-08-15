# flowmcp — MCP server

TypeScript MCP server (protocol **2026-07-28**, stdio) exposing the FlowControl
core to agent hosts. A **thin, stateless translator** over flowd's gRPC: one
channel, request in → gRPC call → result out, no state held between calls. Talks
to `flowd` from the same shared bindings as the web app (`@flow/api`), so the wire
contract stays in one place.

## Architecture

- `@modelcontextprotocol/server` (v2) over **stdio**; `serveStdio` handles protocol
  era negotiation.
- flowd client via `@connectrpc/connect-node` `createGrpcTransport` (native h2c).
- `src/tools/registry.ts` — a **declarative tool registry**. Each tool = a thin
  transport shim (`run`) + a separate **presenter** (grpc result → tool result).
  The registry wraps every tool with uniform error presentation.
- `src/errors.ts` — gRPC code → MCP `isError` tool result with a retry hint
  (timeout-class errors warn "may have completed, verify before re-issuing").
- Discipline: wrapper-minted idempotency keys (`src/meta.ts`), explicit ids,
  per-call deadlines, stderr-only diagnostics, degraded-mode startup.

## Run / test

flowmcp runs on Node natively (no Docker). Its one dependency, flowd, runs in
Docker:

```
docker compose up flowd          # start the core (from repo root)
npm run dev -w flowmcp           # start the MCP server on stdio (FLOWD_ADDR)
npm run typecheck -w flowmcp     # tsc --noEmit
npm run test -w flowmcp          # vitest (unit; typed fakes, no flowd needed)

# End-to-end smoke: a real MCP client ↔ this server ↔ real flowd. Needs flowd up.
docker compose up -d flowd
RUN_INTEGRATION=1 npm run test:integration -w flowmcp
```

## Status

**M3 complete (v1).** 16 tools (`plan/plan.flowmcp.tools.md`), typed-fake unit
tests + a full client↔server↔flowd integration smoke, `mcp` + `mcp-integration`
CI jobs. See `plan/plan.flowmcp.md` and `plan/plan.flowmcp.tools.md`.

Later (M4): MCP resources/prompts, `subscriptions/listen`, streamable HTTP + auth,
multi-tenant hosting.
