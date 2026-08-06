# Protobuf

`proto/fctrl/v1/fctrl.proto`.

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
