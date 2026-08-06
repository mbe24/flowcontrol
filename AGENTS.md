# FlowControl

Multi-interface task management ecosystem for developer-agent collaboration: a Rust core daemon (`flowd`), a Go Bubble Tea CLI (`flowcli`), a TypeScript MCP server (`flowmcp`), and a TypeScript Svelte web app (`flowui`). The gRPC/Protobuf contracts in `proto/` are the single source of truth; generate clients in each language with `protoc` (see `plan/init.md`).

## Commits

Use Conventional Commits with a scope on the enclosing directory. Imperative mood, lowercase start, no trailing period.

- Structure: `type(scope): summary`
- Types: `feat`, `fix`, `chore`, `build`, `docs`, `refactor`, `test`, `perf`
- Scope: the component you touched, e.g. `flowcli`, `flowd`, `flowmcp`, `flowui`, `proto`, `plan`
- Example: `chore(flowcli): add go.sum to fix first build`

## DeepSeek: editing files (only in this harness)

To safely edit text, hold the replacement in a double-quoted here-string (`@"..."@`) and swap it with exact `.Replace(old, new)` behind a `Contains` guard. Caveat: a literal dollar sign must be escaped by a preceding backtick.

Note: may be outdated, in future Codex versions — re-test the here-string approach first.
