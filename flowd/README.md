# Core architecture — the three questions

## 1. Does the core need to be event-based?

Two questions hide inside that one, and they have different answers.

### Does the transport need to push? Yes, and it is not optional.

You have two clients open at once and a data model where one write changes many
rows. Mark T-1042 DONE and three other tasks flip from BLOCKED to READY. If the
TUI has to poll to learn that, then for however long the poll interval is, the
web app and the TUI disagree about what is ready — and "what is ready right now"
is the entire product.

Polling also scales badly in exactly the wrong direction: the fix for staleness
is a shorter interval, which costs you a full graph recomputation per client per
tick, forever, almost always returning "nothing changed".

So: **one server-streaming RPC, `Watch`, held open per client.** Every mutation
returns the events it produced, and those same events arrive on the stream. A
client can apply optimistically and let the stream reconcile, or ignore its own
return value and wait. Both work.

The important detail is that the stream's unit is a **batch, not a node**. A
cascade is one logical change producing many node changes; `WatchResponse`
carries `changed_nodes` so the engine computes the cascade once instead of every
client reimplementing it. That is also what keeps the two clients honest — the
Svelte app and the Go TUI cannot drift on what BLOCKED means if neither computes
it.

### Does the core need event sourcing internally? No — but keep an event log.

Full event sourcing (state is a fold over events, no mutable tables) would be
overkill. You would take on snapshotting, replay performance and schema
evolution to solve problems a single-writer local database does not have.

But an **append-only `events` table beside normal mutable state** is worth it
here, because in this system one table does three jobs at once:

1. the activity feed the clients already render,
2. the change stream `Watch` replays from,
3. the undo log.

You were going to build the first one anyway. The other two come free. `seq` is
monotonic, so a reconnecting client sends its last seq and gets the gap — or a
`resync_required` if the gap is too wide. That is the whole catch-up protocol,
and it fits in one integer.

So the answer is: **event-driven at the boundary, ordinary relational state
underneath, with an append-only log that serves the feed, the stream and undo.**

### Transport choice

gRPC over a Unix domain socket (or a named pipe on Windows) rather than TCP: you
get the streaming, the generated clients for Rust, TypeScript and Go, and no
open port. Use `tonic` on the Rust side. The web app cannot speak raw gRPC from
a browser, so put `grpc-web` or Connect in front of it — the proto is unchanged
either way.

---

## 2. Database schema — plain SQL, not an ORM

`migrations/0001_initial_schema.sql`, SQLite, designed for `sqlx`.

**Why not Diesel or SeaORM.** The interesting queries here are recursive graph
walks: cycle detection, longest chain, hops-from-ready. Both ORMs make you drop
to raw SQL for those, so the ORM buys you nothing but a second place for the
schema to live and a mapping layer to keep in sync. `sqlx` checks your SQL
against the real database at compile time, which is the guarantee you actually
wanted.

**Why SQLite.** Single-writer, local, embedded in the core process. Turn on WAL
and readers never block the writer. If this ever becomes multi-user the schema
ports to Postgres with small changes (`unixepoch()`, `STRICT`, FTS5).

Three decisions in that file worth arguing about:

**READY and BLOCKED are not stored.** `nodes.declared_status` is only ever
`OPEN`, `DEFERRED` or `DONE` — what a human or agent actually set. READY and
BLOCKED are derived in the `node_state` view. Storing them would give you two
sources of truth for the same fact and force every write path to maintain the
cascade. The derivation is also cheaper than it looks: a node is blocked if any
*direct* blocker is not DONE, and that blocker's own blockers are its problem.
One join, no fixpoint.

**Verification is its own table, and it is two independent halves.**
`agent_result` is what was reported; `human_verdict` is your acceptance. They
are stored separately because the interesting case — accepting over a reported
failure — needs both to survive. `agent_node_rev` records the node revision at
report time, so "the report is stale" is computed rather than guessed.

**Cycles are rejected at write time by a trigger**, not validated in application
code. Any path that inserts an edge gets the check, including a future agent
writing through MCP.

---

## 3. Protobuf

`../proto/flow/v1/flow.proto`.

Deliberate choices:

- **`DeclaredStatus` and `EffectiveStatus` are different enums.** You cannot ask
  the core to set a node READY — that is the engine's answer, not the client's
  request. The type system says so.
- **Every mutation returns `MutationResponse`**: the events, the changed nodes,
  the changed progress, the new seq. Same shape as a `WatchResponse` body, so
  clients have one apply-path for both.
- **`WriteMeta` carries an idempotency key.** Agents retry. Without it, a
  retried `SetStatus` writes two events and undo does the wrong thing.
- **`ReportCondition` is separate from `SetVerdict`** — different callers,
  different meaning, different auth story later.
- **`Progress` is computed server-side.** Two clients rendering the same ratio
  bar must not disagree about what 44% means.
- **`GetSnapshot` returns a `seq`.** Snapshot then `Watch(from_seq)` is
  race-free: everything in the snapshot is consistent as of that cursor.

## Suggested layout

```
flowcontrol/
  flowd/                   Rust — tonic + sqlx
    migrations/             .sql, checked into git
  proto/flow/v1/
  flowcli/                    Go TUI       → generated client
  flowui/                Svelte app   → generated client via grpc-web
```

## What I would build first

1. Schema + `sqlx` wiring, no server. Prove the views with fixture data.
2. Unary RPCs only. Point the Go TUI at it — it already has a `Store` interface
   with exactly these methods.
3. `Watch`. Now the two clients stay in sync and both can drop their fixtures.
4. Undo, using the `payload` column you have been writing all along.
