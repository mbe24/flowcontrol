// Public API of flowdjs for other workspaces (flowmcp, and later flowcli's build
// tooling) — ensure/discover the shared daemon without reaching into internals.
// The daemon entry (index.ts) and server remain private.
export { ensureDaemon, isAnotherDaemonLive, probe, readSession } from "./session";
export type { Session, SpawnedBy } from "./session";
