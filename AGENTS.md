# FlowControl

Multi-interface task management ecosystem for developer-agent collaboration: a Rust core daemon (`flowd`), a Go Bubble Tea CLI (`fctrl`), a TypeScript MCP server (`fctrl-mcp`), and a TypeScript Svelte web app (`fctrl-web`). The gRPC/Protobuf contracts in `proto/` are the single source of truth; generate clients in each language with `protoc` (see `plan/init.md`).

## Commits

Use Conventional Commits with a scope on the enclosing directory. Imperative mood, lowercase start, no trailing period.

- Structure: `type(scope): summary`
- Types: `feat`, `fix`, `chore`, `build`, `docs`, `refactor`, `test`, `perf`
- Scope: the component you touched, e.g. `fctrl`, `flowd`, `fctrl-mcp`, `fctrl-web`, `proto`, `plan`
- Example: `chore(fctrl): add go.sum to fix first build`

## DeepSeek: editing files (only in this harness)

In this current Codex harness, `apply_patch` fails on multi-line patches (the end marker is lost through the Windows argv shim). Edit via a small `.ps1` script using `[System.IO.File]::WriteAllLines/WriteAllText`; build multi-line strings with `[char]10`; use exact-string `.Replace()` with a `Contains` guard (never delete by line index); re-verify after edits; and delete the script after.

Note: this applies to the current harness and may be outdated for future versions — re-test `apply_patch` before relying on it.
