# Concepts

## The hierarchy

flowcontrol has one fixed shape:

| Kind | Parent | Purpose |
| --- | --- | --- |
| **Work package** | — (top level) | a body of work; groups tasks |
| **Task** | a work package | a unit of work with a verifiable **condition** |
| **Step** | a task | the granular thing an agent executes and checks off |

Everything lives inside a **project** — the namespace that holds the whole tree.

## Dependencies

Any node can block any other node **across levels** — a step can block a whole work package.
A dependency edge means _the blocker must be `DONE` before the blocked node can become
workable_. Cycles are rejected.

## Declared vs. effective status

Every node carries a **declared** status you set, and an **effective** status the engine
computes from the graph. You only ever set the declared one:

| You set (declared) | meaning |
| --- | --- |
| `OPEN` | workable — hand it to the engine |
| `DEFERRED` | paused on purpose |
| `DONE` | complete |

The engine derives the effective status — you cannot set these directly:

| Effective | meaning |
| --- | --- |
| 🟢 **READY** | every blocker is `DONE` — workable now |
| 🔴 **BLOCKED** | still waiting on a blocker |
| ⚪ **DEFERRED** | you paused it |
| 🔵 **DONE** | complete — marking it re-evaluates everything downstream |

That single rule — **mark one node `DONE`, the cascade re-flows** — is the whole idea, and it
is what the logo shows: one filled node releasing two hollow ones.

## Conditions

A task carries a free-text **condition** — a verifiable check (often a command), e.g.
`pnpm --filter flowmcp test passes`. It records _how you know the task is truly done_, and can
be confirmed later with the `report_condition` tool.

## Projects, comments, events, undo

- **Projects** partition the graph; a single daemon can hold many.
- **Comments** attach discussion to any node.
- **Events** are an append-only log of every change, with a `seq` cursor so clients can ask
  "what changed since?" (`poll_changes`).
- **Undo** reverts the last change.
