# flowcli — terminal UI

Go + Bubble Tea prototype of the FlowControl TUI. Fixture data only; no engine
attached. Screens follow `FlowControl TUI v2.dc.html`.

## Run

```
cd flowcontrol/flowcli
go mod tidy
go run .
```

Written without a compiler in the loop — expect to fix a nit or two on first
build.

## Layout

```
main.go                      wires the store to the program
internal/store/store.go      types, Store interface, Verification.Badge()
internal/store/memory.go     fixture project — same data as the Svelte app
internal/styles/styles.go    the one palette, matching the web tokens
internal/ui/model.go         state, indexing, tree flattening
internal/ui/update.go        key handling per screen and overlay
internal/ui/view.go          frame helper, tree, detail, activity
internal/ui/lanes.go         lane layout and width detection
internal/ui/chain.go         dependency spine and focus mode
internal/ui/overlays.go      finder, projects, status, confirm, comment
```

## Screens

| key | screen |
| --- | --- |
| `1` | tree browser |
| `2` | lanes |
| `3` | chain |
| `ret` | detail (from any list) |
| `a` | activity, from detail |
| `/` | finder · `p` projects — overlay any screen |

Other keys: `j/k` move, `h/l` fold or change lane, `tab` expand a step,
`s` status picker, `v` toggle the verification flag, `f` focus a chain,
`w` next work package, `u` undo the last status change, `D` show done packages,
`esc` back one layer, `q` quit.

## Conditions are reported, not run

There is no `Verify` on the store. An agent runs the condition and reports;
`SetVerdict` records your acceptance.

```go
type AgentResult  string // pass | fail | stale | none
type HumanVerdict string // accepted | rejected | none
```

`Verification.Badge()` resolves the pair into one display state — the human
verdict wins, the agent's report stays visible beside it. Pressing `v` when the
agent reported `fail` opens a confirm prompt; agreeing with a pass is silent.
Steps show their condition text but carry no flag: acceptance is task-level.

## Lanes

Thresholds are derived from the card widths, not chosen separately:

```
≥100 cols   four lanes    4×22 + gutters + frame = 98
 68–99      two lanes     2×30 + gutter + frame = 67   ← the default
 44–67      one lane + tab strip
 <44        falls back to the tree
```

Every cell is hard-truncated to the lane width, so no cell can push its
neighbour. Titles that do not fit end in `-` — visible truncation rather than
silent. The `←blocker` annotation is dropped below 30 columns.

## Chain instead of a 2D graph

`buildChain` walks the dependency graph and emits a git-log style spine: one row
per task, indentation from the longest chain of blockers above it. Status is the
dot's colour and nothing else — ratios, conditions and verification live in the
detail view.

Cross-package and cross-level blockers become a `╎ also waits on …` annotation
rather than a drawn edge, which is the same rollup the web graph shows as a
dashed package edge.

`f` narrows to the ancestors and descendants of the selected node, with hops-
from-ready and longest-chain counts.

## The seam

```go
p := tea.NewProgram(ui.New(store.NewMemory()))
```

Replace `store.NewMemory()` with a client implementing `store.Store` — seven
methods, all returning errors already.

## What's faked

- **Cascade** — `SetStatus` writes one node. The engine recomputes downstream
  BLOCKED/READY.
- **Undo** — one level, in memory.
- **Editing** — titles, conditions and new tasks are inert.
- **Timestamps** — fixture strings ("2d ago"), not real times.
- **Scrolling** — long lists clip at the frame rather than scrolling; the
  viewport wiring is the obvious next commit.

## Additions to the data model

Same three as the web app, none in `plan/datamodel.md` v1.0: work-package
`State`, per-task `Verification`, step `Note`, plus `ActivityEntry` for the
history and comment stream.
