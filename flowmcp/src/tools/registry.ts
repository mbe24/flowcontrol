import type { CallToolResult, McpServer } from "@modelcontextprotocol/server";
import type { ZodType } from "zod";
import type { FlowClient } from "../flowd";
import { presentError } from "../errors";

/** A tool's arg schema: a map of field name → zod validator (SDK `ZodRawShape`). */
export type Shape = Record<string, ZodType>;

/** Shared dependencies handed to every tool. */
export interface ToolDeps {
  flow: FlowClient;
  /** Per-call gRPC deadline (ms) so a wedged flowd can't hang the host forever. */
  callTimeoutMs: number;
  /** Display label written to `WriteMeta.author` (never an identity). */
  author: string;
}

/**
 * A tool definition. `run` is the thin transport shim: call flowd, then map to a
 * tool result via a per-tool *presenter* (a separate pure function). Keeping `run`
 * free of error handling is the seam the §6 tool-design pass reshapes — the
 * registry wraps every tool with uniform error presentation.
 */
export interface ToolDef {
  name: string;
  title?: string;
  description: string;
  inputSchema: Shape;
  outputSchema?: Shape;
  run(args: Record<string, unknown>, deps: ToolDeps): Promise<CallToolResult>;
}

/** Register a declarative tool set on the server; `tools/list` derives from it. */
export function registerTools(
  server: McpServer,
  deps: ToolDeps,
  defs: ToolDef[],
): void {
  for (const def of defs) {
    server.registerTool(
      def.name,
      {
        ...(def.title ? { title: def.title } : {}),
        description: def.description,
        inputSchema: def.inputSchema,
        ...(def.outputSchema ? { outputSchema: def.outputSchema } : {}),
      },
      // The SDK validates args against inputSchema before calling; run() re-shapes.
      async (args: unknown): Promise<CallToolResult> => {
        try {
          return await def.run(args as Record<string, unknown>, deps);
        } catch (err) {
          return presentError(err);
        }
      },
    );
  }
}
