import type { ActivityEntry, Dependency, FlowNode, HumanVerdict, Project, Status } from './types';

/**
 * The seam between the UI and the engine. MemoryStore implements it now; a
 * client that talks to the Rust core over gRPC or a named pipe implements the
 * same methods later and nothing under src/components changes.
 *
 * Note there is no `verify()`. fctrl does not run conditions — agents run them
 * and report; `setVerdict` records the human's acceptance.
 */
export interface FlowStore {
  projects(): Promise<Project[]>;
  nodes(projectId: string): Promise<FlowNode[]>;
  dependencies(projectId: string): Promise<Dependency[]>;
  activity(projectId: string): Promise<ActivityEntry[]>;
  /** Writes one node. The engine owns the downstream cascade. */
  setStatus(nodeId: string, status: Status): Promise<void>;
  /** Records the human's acceptance or rejection of a condition. */
  setVerdict(nodeId: string, verdict: HumanVerdict): Promise<void>;
  addComment(nodeId: string, text: string): Promise<void>;
}
