// In-process fan-out for Watch. `dispatch` is unary-only (a deliberate core/edge
// split — see flowcore), so the daemon publishes each committed mutation here and
// Watch streams subscribe. Mirrors the native daemon's Watch notifier.
import type { Mutation } from "@flow/api/flow/v1/flow_pb";

export type Listener = (projectId: string, mutation: Mutation) => void;

export class Bus {
  private listeners = new Set<Listener>();

  publish(projectId: string, mutation: Mutation): void {
    for (const l of this.listeners) l(projectId, mutation);
  }

  add(l: Listener): void {
    this.listeners.add(l);
  }

  remove(l: Listener): void {
    this.listeners.delete(l);
  }
}
