# FlowControl

Multi-interface task management ecosystem for developer-agent collaboration: a Rust core daemon (`fctrld`), a Go Bubble Tea CLI (`fctrl`), a TypeScript MCP server (`fctrl-mcp`), and a TypeScript Svelte web app (`fctrl-web`). The gRPC/Protobuf contracts in `proto/` are the single source of truth; generate clients in each language with `protoc` (see `plan/init.md`).

## Commits

Use Conventional Commits with a scope on the enclosing directory. Imperative mood, lowercase start, no trailing period.

- Structure: `type(scope): summary`
- Types: `feat`, `fix`, `chore`, `build`, `docs`, `refactor`, `test`, `perf`
- Scope: the component you touched, e.g. `fctrl`, `fctrld`, `fctrl-mcp`, `fctrl-web`, `proto`, `plan`
- Example: `chore(fctrl): add go.sum to fix first build`
