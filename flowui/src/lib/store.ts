import type { ActivityEntry, Dependency, FlowNode, HumanVerdict, NodeType, Project, Status } from './types';

/** Input needed to create a node (a work package, task or step). */
export interface CreateNodeInput {
  projectId: string;
  /** Parent id; empty for a work package. */
  parentId: string | null;
  kind: NodeType;
  title: string;
  description?: string;
  condition?: string;
}

/** Editable node fields. Only the fields provided are written (update mask). */
export interface UpdateNodeInput {
  title?: string;
  description?: string;
  condition?: string;
  reference?: string;
}

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
  /** Creates a node; returns its id. */
  createNode(input: CreateNodeInput): Promise<string>;
  /** Updates editable fields of a node (only provided fields are written). */
  updateNode(nodeId: string, patch: UpdateNodeInput): Promise<void>;
  /** Deletes a node and its subtree. */
  deleteNode(nodeId: string): Promise<void>;
  /** Adds a dependency edge. */
  addDependency(blockerId: string, blockedId: string): Promise<void>;
  /** Removes a dependency edge. */
  removeDependency(blockerId: string, blockedId: string): Promise<void>;
  /** Reverses the most recent event for a project (server-side undo). */
  undo(projectId: string): Promise<void>;
}
