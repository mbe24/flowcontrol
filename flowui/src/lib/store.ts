import type {
  ActivityEntry,
  Dependency,
  FlowNode,
  HumanVerdict,
  NodeType,
  Project,
  Status,
  WPState
} from './types';

export interface NewNode {
  projectId: string;
  parentId: string | null;
  type: NodeType;
  title: string;
  description?: string[];
  condition?: string;
  /** STEP only — its body/detail, same field the detail pane edits later. */
  note?: string;
}

export interface NodePatch {
  title?: string;
  description?: string[];
  condition?: string;
  /** STEP only — its body/detail (a few sentences, shown on expand). */
  note?: string;
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
  /** Work-package lifecycle. Separate from Status, which packages don't use. */
  setWpState(nodeId: string, state: WPState): Promise<void>;
  addComment(nodeId: string, text: string): Promise<void>;

  createNode(input: NewNode): Promise<string>;
  updateNode(nodeId: string, patch: NodePatch): Promise<void>;
  deleteNode(nodeId: string): Promise<void>;
  /**
   * Promote, demote, reparent and reorder are all the same write. Passing a
   * different `type` is what makes it a promote or a demote.
   */
  moveNode(nodeId: string, newParentId: string, newType: NodeType): Promise<void>;

  addDependency(blockerId: string, blockedId: string): Promise<void>;
  removeDependency(blockerId: string, blockedId: string): Promise<void>;

  createProject(name: string, description: string, seedWorkPackage: boolean): Promise<string>;
  updateProject(projectId: string, patch: { name?: string; description?: string }): Promise<void>;
  archiveProject(projectId: string, archived: boolean): Promise<void>;
}
