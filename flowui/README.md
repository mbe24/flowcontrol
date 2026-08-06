# flowui — Svelte prototype

Svelte 5 + Vite + TypeScript prototype of the web app designed in
`FlowControl.dc.html`. Fixture data only; no engine attached.

## Run

```
cd flowcontrol/flowui
npm install
npm run dev
```

`npm run check` runs svelte-check. Written without a build in the loop — expect
to fix a nit or two on first run.

## Layout

```
index.html                     fonts + #app
src/main.ts                    mount
src/app.css                    the design tokens (dark + light) and resets
src/lib/types.ts               data model types, status hues, glyphs
src/lib/store.ts               FlowStore interface — the seam
src/lib/memory.ts              fixture project, same data as the Go TUI
src/lib/derive.ts              indexing, counts, and the graph layout
src/lib/state.svelte.ts        app state ($state runes) + actions
src/App.svelte                 shell, view routing, ⌘K
src/components/Rail.svelte     project switcher + theme toggle
src/components/TopBar.svelte   view tabs, project progress, search
src/components/FilterBar.svelte  status filter chips
src/components/TableView.svelte  hierarchical table with progressive disclosure
src/components/LanesView.svelte  status lanes, colour = work package
src/components/GraphView.svelte  clusters, collapsed boxes, rollup edges
src/components/DetailPanel.svelte  detail, condition + verify, steps, deps
src/components/Palette.svelte    ⌘K command palette
```

## No component library

Plain Svelte components with scoped CSS over CSS custom properties. shadcn-svelte
would mean Tailwind plus a generator for a prototype that needs eight components,
and the design isn't built on shadcn's primitives — the tokens in `app.css` are.
If you do adopt shadcn-svelte later, `app.css` maps cleanly onto its CSS variable
convention.

## The seam

`FlowStore` is five async methods. `MemoryStore` implements it now; swap the one
line in `state.svelte.ts`:

```ts
export const store: FlowStore = new MemoryStore();
```

Every call already goes through `await`, so a transport with real latency won't
force a restructure. `MemoryStore` sleeps 20–900ms deliberately.

## Interactions that work

- Table / Lanes / Graph tabs (or keys `1` `2` `3`), sharing one selection.
- Work-package and task expand/collapse; done packages auto-collapse behind a
  disclosure row.
- Status filter chips; click a row, card or graph node to select it.
- Detail panel: set status (writes through the store), run `verify`, `undo` the
  last change.
- ⌘K palette over tasks, steps and status commands.
- Dark/light toggle in the rail.
- Graph edit mode reveals connection ports (visual only — no dragging).

## The graph layout is real

`layoutGraph` in `derive.ts` computes it from the data rather than hard-coding
pixels like the mockup did:

- Work packages with `state: 'ACTIVE'` become expanded clusters; the rest are
  collapsed boxes down the left.
- Inside a cluster, a task's column is the longest chain of intra-package
  blockers above it, so dependencies flow left to right.
- Any dependency it cannot draw task-to-task — cross-level, or into a collapsed
  package — rolls up into one dashed package edge badged `via N task deps`.
- Edges leave and enter on facing edges of the two rectangles.

Comfortable ceiling is 3–4 expanded clusters. Beyond that it needs virtualisation
and a proper force or layered layout (elkjs, dagre) rather than this.

## What's faked

- **Verify** — `MemoryStore.verify` sleeps 900ms and returns a canned result.
- **Cascade** — `setStatus` writes one node. The engine recomputes downstream
  BLOCKED/READY; the prototype does not pretend to.
- **Undo** — one level, in memory.
- **Editing** — titles, conditions, new tasks and edge dragging are all inert.
- **Zoom** — the graph toolbar's zoom control is decoration; the canvas scrolls.

## Additions to the data model

Both also in the Go TUI, neither in `plan/datamodel.md` v1.0:

- `state` on work packages: `PLANNED | ACTIVE | DONE | ARCHIVED`.
- `lastResult` / `lastRun` per node: cached verification of `condition`.
