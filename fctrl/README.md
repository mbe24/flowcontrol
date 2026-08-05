# fctrl — TUI prototype

Bubble Tea prototype of the terminal client designed in `flowcontrol-tui.dc.html`.
Fixture data only; no engine attached.

## Run

```
cd flowcontrol/fctrl
go mod tidy      # resolves bubbletea / bubbles / lipgloss and writes go.sum
go run .
```

Written without a compiler in the loop — expect to fix a nit or two on first
build.

## Layout

```
main.go                     wires a Store into the program
internal/store/store.go     types + the Store interface (the seam)
internal/store/memory.go    fixture project, mirrors the mockups
internal/styles/styles.go   the four status hues, glyphs, progress bar
internal/ui/model.go        state, row building, indexing, search
internal/ui/update.go       every keybinding
internal/ui/view.go         header, tree pane, detail pane, status line
internal/ui/overlays.go     finder, status picker, confirm bar, projects, help
```

## The seam

`store.Store` is five methods. `Memory` implements it now; a named-pipe or gRPC
client implements the same interface later and nothing under `internal/ui`
changes. Swap the one line in `main.go`.

Loads already go through `tea.Cmd`s (`loadProjects`, `loadData`, `applyStatus`,
`verifyNode`), so a remote store that blocks for 20ms won't need the UI
restructured.

## What's faked

- **Verify** — `Memory.Verify` sleeps 900ms and returns a canned result. The
  spinner and the persistent result line are real.
- **Cascade** — `SetStatus` writes the one node. The confirm bar lists the
  dependents the engine *would* re-evaluate and says so, rather than
  half-implementing DAG logic the Rust core owns.
- **Undo** — `u` restores the previous status of the last node you changed. One
  level, in memory.
- **Edit** — `e` is unbound. Titles and conditions are read-only.
- **Finder overlay** — centred over the body instead of dimming the tree behind
  it; Bubble Tea has no compositor, so a real dim needs rendering the tree to a
  string and overwriting a rect.

## Deviations from the mockups

- Status picker and confirm bar render *inline*, overwriting rows below the
  cursor, rather than as a floating box.
- The work-package `state` field (`PLANNED`/`ACTIVE`/`DONE`/`ARCHIVED`) and the
  per-node cached verification result are additions to the v1.0 data model —
  the TUI needs both and they aren't in `plan/datamodel.md` yet.
- Narrow mode (<100 columns) drops to a single pane; `tab`/`esc` push and pop.

## Keys

`j/k` move · `g/G` jump · `space` fold · `↵` open · `tab` pane · `/` or `ctrl-p`
finder · `s` status · `d` done · `v` verify · `u` undo · `a` show done packages ·
`p` projects · `?` keymap · `q` quit
