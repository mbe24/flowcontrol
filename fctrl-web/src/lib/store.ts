import type {
  Dependency,
  FlowNode,
  Project,
  Status,
  VerifyResult,
} from './types';

/**
 * The seam between the UI and the engine. MemoryStore implements it now; a
 * client that talks to the Rust core over gRPC or a named pipe implements the
 * same five methods later and nothing under src/components changes.
 */
export interface FlowStore {
  projects(): Promise<Project[]>;
  nodes(projectId: string): Promise<FlowNode[]>;
  dependencies(projectId: string): Promise<Dependency[]>;
  /** Writes one node. The engine owns the downstream cascade. */
  setStatus(nodeId: string, status: Status): Promise<void>;
  /** Runs the node's condition and returns the outcome. */
  verify(nodeId: string): Promise<VerifyResult>;
}
