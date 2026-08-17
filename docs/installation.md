# Installation

!!! note "Pre-release"
    flowcontrol is not on npm yet, so today you run it from the repository. The npm one-liners
    below are the intended surface once published.

## Run from source

Requires **Node ≥ 22.5**, **pnpm** (via `corepack`), and a Rust toolchain (or Docker) to
generate the WebAssembly engine.

```sh
git clone https://github.com/mbe24/flowcontrol
cd flowcontrol

corepack pnpm install
corepack pnpm gen:wasm               # build the flowcore wasm
corepack pnpm --filter flowui build  # build the web board (daemon-connected)
corepack pnpm --filter flowdjs ui    # start the daemon + open the board in your browser
```

The last command starts a machine-local daemon (writing `~/.flowcontrol/session.json` and a
SQLite file) and opens the web board pointed at it.

## Wire up your agent (MCP)

Point any MCP client — Claude Code, Claude Desktop, Cursor, … — at the from-source server:

```json
{
  "mcpServers": {
    "flowcontrol": {
      "command": "node",
      "args": ["--import", "tsx", "<path>/flowcontrol/flowmcp/src/index.ts"]
    }
  }
}
```

The MCP connects to the **same** machine-local daemon the board uses, so tasks your agent
creates appear on the board immediately. Discovery and the bearer token are shared through
`~/.flowcontrol/session.json` — nothing to copy.

!!! tip "One daemon per machine"
    State is per-machine (`~/.flowcontrol`); flowcontrol's own `project` entity partitions the
    graph. Whichever client you open first starts the daemon; the rest connect to it.

## Coming soon (npm)

Once published, each front door is a single command — no toolchain required:

```sh
npm i -g @mbe24/flowmcp @mbe24/flowcli   # the agent server + the terminal UI
flowui                                    # open the web board (starts the daemon)
```

A `flowmcp install` command will write the MCP config into your agent host automatically.
