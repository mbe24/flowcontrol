import { Code, ConnectError } from "@connectrpc/connect";
import type { CallToolResult } from "@modelcontextprotocol/server";

/** Transient codes worth an automatic same-key retry (handled by the wrapper). */
export function isRetryable(code: Code): boolean {
  return (
    code === Code.Unavailable ||
    code === Code.DeadlineExceeded ||
    code === Code.Aborted
  );
}

/**
 * Turn any thrown error into an MCP tool RESULT (`isError`), never a JSON-RPC
 * protocol error — so the model reads the reason and can self-correct. Timeout /
 * unavailable errors carry the "may have already completed, verify first" warning:
 * a *model* retry mints a NEW idempotency key and could double-write.
 */
export function presentError(err: unknown): CallToolResult {
  const ce = ConnectError.from(err);
  const name = Code[ce.code] ?? String(ce.code);
  const lines = [`Error (${name}): ${ce.rawMessage}`];
  if (ce.code === Code.DeadlineExceeded || ce.code === Code.Unavailable) {
    lines.push(
      "This may be transient. The operation may have already completed on the server — " +
        "verify with get_snapshot / poll_changes before re-issuing (a re-call is a NEW operation).",
    );
  } else {
    lines.push("This is a request problem; fix the arguments rather than retrying.");
  }
  return {
    isError: true,
    content: [{ type: "text", text: lines.join("\n") }],
    structuredContent: {
      code: name,
      retryable: isRetryable(ce.code),
      message: ce.rawMessage,
    },
  };
}
