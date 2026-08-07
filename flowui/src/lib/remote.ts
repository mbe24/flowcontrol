import { create } from '@bufbuild/protobuf';
import { createClient, type Client } from '@connectrpc/connect';
import { createGrpcWebTransport } from '@connectrpc/connect-web';
import type { CreateNodeInput, FlowStore, UpdateNodeInput } from './store';
import type {
  ActivityEntry,
  ActivityKind,
  AgentResult,
  Dependency,
  FlowNode,
  HumanVerdict,
  NodeType,
  Project,
  Status,
  Verification,
  WPState
} from './types';
import {
  AddCommentRequestSchema,
  AddDependencyRequestSchema,
  AgentResult as PbAgentResult,
  CreateNodeRequestSchema,
  DeclaredStatus as PbDeclared,
  DeleteNodeRequestSchema,
  EffectiveStatus as PbEffective,
  EventKind as PbEventKind,
  FlowService,
  GetSnapshotRequestSchema,
  HumanVerdict as PbVerdict,
  ListProjectsRequestSchema,
  NodeKind as PbKind,
  RemoveDependencyRequestSchema,
  SetStatusRequestSchema,
  SetVerdictRequestSchema,
  UndoRequestSchema,
  UpdateNodeRequestSchema,
  WorkPackageState as PbWpState,
  type Dependency as PbDependency,
  type Event as PbEvent,
  type Node as PbNode,
  type Project as PbProject
} from '@flow/api/flow/v1/flow_pb';
/** Who writes from the web app. Plain name, agents get no special treatment. */
const AUTHOR = 'you';

/** Localhost (loopback) where `flowd serve` listens by default. */
const DEFAULT_BASE_URL = 'http://127.0.0.1:50051';

function baseUrl(): string {
  return import.meta.env.VITE_SERVER_URL || DEFAULT_BASE_URL;
}

function idemKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

// ── display helpers ─────────────────────────────────────────────────────────

/** Roughly when a Unix-seconds timestamp was, as the UI shows it. */
export function relative(unixSeconds: number): string {
  if (!unixSeconds) return '';
  const s = Math.max(0, Date.now() / 1000 - unixSeconds);
  if (s < 45) return 'just now';
  if (s < 3600) return `${Math.max(1, Math.round(s / 60))}m ago`;
  if (s < 86400) return `${Math.round(s / 3600)}h ago`;
  if (s < 86400 * 14) return `${Math.round(s / 86400)}d ago`;
  return `${Math.round(s / (86400 * 7))}w ago`;
}

function kindOf(k: PbKind): NodeType {
  switch (k) {
    case PbKind.WORK_PACKAGE:
      return 'WORK_PACKAGE';
    case PbKind.TASK:
      return 'TASK';
    default:
      return 'STEP';
  }
}

function kindFor(k: NodeType): PbKind {
  switch (k) {
    case 'WORK_PACKAGE':
      return PbKind.WORK_PACKAGE;
    case 'TASK':
      return PbKind.TASK;
    default:
      return PbKind.STEP;
  }
}

function statusOf(s: PbEffective): Status {
  switch (s) {
    case PbEffective.READY:
      return 'READY';
    case PbEffective.BLOCKED:
      return 'BLOCKED';
    case PbEffective.DEFERRED:
      return 'DEFERRED';
    case PbEffective.DONE:
      return 'DONE';
    default:
      return 'READY';
  }
}

function wpStateOf(s: PbWpState): WPState {
  switch (s) {
    case PbWpState.PLANNED:
      return 'PLANNED';
    case PbWpState.ACTIVE:
      return 'ACTIVE';
    case PbWpState.DONE:
      return 'DONE';
    case PbWpState.ARCHIVED:
      return 'ARCHIVED';
    default:
      return 'PLANNED';
  }
}

function verificationOf(v: PbNode['verification']): Verification | undefined {
  if (!v) return undefined;
  const agent: AgentResult = v.stale ? 'stale' : v.agentResult === PbAgentResult.PASS ? 'pass' : v.agentResult === PbAgentResult.FAIL ? 'fail' : 'none';
  let human: HumanVerdict = 'none';
  if (v.humanVerdict === PbVerdict.ACCEPTED) human = 'accepted';
  else if (v.humanVerdict === PbVerdict.REJECTED) human = 'rejected';
  const has = agent !== 'none' || human !== 'none';
  if (!has) return undefined;
  return {
    agent,
    agentName: v.agentName || '',
    agentWhen: relative(Number(v.agentAt)),
    human,
    humanWhen: human === 'none' ? '' : relative(Number(v.humanAt))
  };
}

// ── proto → UI mapping (pure, unit-testable) ────────────────────────────────

export function mapProject(p: PbProject): Project {
  return {
    id: p.id,
    name: p.name,
    description: p.description,
    createdAt: Number(p.createdAt)
  };
}

export function mapNode(n: PbNode): FlowNode {
  const node: FlowNode = {
    id: n.id,
    projectId: n.projectId,
    parentId: n.parentId || null,
    type: kindOf(n.kind),
    title: n.title,
    description: n.description ? n.description.split(/\n+/).filter((s) => s.trim()) : [],
    status: statusOf(n.status)
  };
  if (n.condition) node.condition = n.condition;
  if (n.kind === PbKind.WORK_PACKAGE && n.wpState) node.state = wpStateOf(n.wpState);
  const v = verificationOf(n.verification);
  if (v) node.verification = v;
  return node;
}

export function mapDependency(d: PbDependency): Dependency {
  return { blockerId: d.blockerId, blockedId: d.blockedId };
}

export function mapActivity(e: PbEvent): ActivityEntry {
  let kind: ActivityKind = 'edit';
  switch (e.kind) {
    case PbEventKind.STATUS_SET:
      kind = 'status';
      break;
    case PbEventKind.AGENT_REPORTED:
    case PbEventKind.VERDICT_SET:
      kind = 'verify';
      break;
    case PbEventKind.COMMENT:
      kind = 'comment';
      break;
    default:
      kind = 'edit';
  }
  return {
    id: String(e.seq),
    nodeId: e.nodeId,
    kind,
    author: e.author,
    when: relative(Number(e.createdAt)),
    text: e.summary
  };
}

// ── UI → proto writes ───────────────────────────────────────────────────────

/**
 * The UI works in effective statuses, but a client may only *declare* OPEN,
 * DEFERRED or DONE (READY/BLOCKED are the engine's answer). "Set to READY or
 * BLOCKED" therefore means "open" — the engine re-derives the effective state.
 */
export function toDeclaredStatus(status: Status): PbDeclared {
  switch (status) {
    case 'DONE':
      return PbDeclared.DONE;
    case 'DEFERRED':
      return PbDeclared.DEFERRED;
    default:
      return PbDeclared.OPEN;
  }
}

export function toVerdict(verdict: HumanVerdict): PbVerdict {
  switch (verdict) {
    case 'accepted':
      return PbVerdict.ACCEPTED;
    case 'rejected':
      return PbVerdict.REJECTED;
    default:
      return PbVerdict.UNSPECIFIED;
  }
}

// ── the remote store ────────────────────────────────────────────────────────

/** Just the RPCs RemoteStore uses, so tests can inject a fake. */
type FlowServiceShim = Pick<
  Client<typeof FlowService>,
  | 'listProjects'
  | 'getSnapshot'
  | 'setStatus'
  | 'setVerdict'
  | 'addComment'
  | 'createNode'
  | 'updateNode'
  | 'deleteNode'
  | 'addDependency'
  | 'removeDependency'
  | 'undo'
>;

/**
 * FlowStore backed by the FlowControl core over grpc-web. Kept transport
 * agnostic in name so a later native-pipe client can slot in behind the same
 * seam; swapped in for MemoryStore by the env-flag selector in state.svelte.ts.
 */
export class RemoteStore implements FlowStore {
  private client: FlowServiceShim;

  constructor(client?: FlowServiceShim) {
    this.client =
      client ??
      createClient(
        FlowService,
        createGrpcWebTransport({ baseUrl: baseUrl(), useBinaryFormat: true })
      );
  }

  private async snapshot(projectId: string) {
    return this.client.getSnapshot(create(GetSnapshotRequestSchema, { projectId }));
  }

  async projects(): Promise<Project[]> {
    const res = await this.client.listProjects(
      create(ListProjectsRequestSchema, { includeArchived: true })
    );
    return res.projects.map(mapProject);
  }

  async nodes(projectId: string): Promise<FlowNode[]> {
    const snap = await this.snapshot(projectId);
    return snap.nodes.map(mapNode);
  }

  async dependencies(projectId: string): Promise<Dependency[]> {
    const snap = await this.snapshot(projectId);
    return snap.dependencies.map(mapDependency);
  }

  async activity(projectId: string): Promise<ActivityEntry[]> {
    const snap = await this.snapshot(projectId);
    return snap.recentEvents.map(mapActivity);
  }

  async setStatus(nodeId: string, status: Status): Promise<void> {
    await this.client.setStatus(
      create(SetStatusRequestSchema, {
        meta: { author: AUTHOR, idempotencyKey: idemKey() },
        nodeId,
        declaredStatus: toDeclaredStatus(status)
      })
    );
  }

  async setVerdict(nodeId: string, verdict: HumanVerdict): Promise<void> {
    await this.client.setVerdict(
      create(SetVerdictRequestSchema, {
        meta: { author: AUTHOR, idempotencyKey: idemKey() },
        nodeId,
        verdict: toVerdict(verdict)
      })
    );
  }

  async addComment(nodeId: string, text: string): Promise<void> {
    await this.client.addComment(
      create(AddCommentRequestSchema, {
        meta: { author: AUTHOR, idempotencyKey: idemKey() },
        nodeId,
        text
      })
    );
  }

  async createNode(input: CreateNodeInput): Promise<string> {
    const res = await this.client.createNode(
      create(CreateNodeRequestSchema, {
        meta: { author: AUTHOR, idempotencyKey: idemKey() },
        projectId: input.projectId,
        parentId: input.parentId ?? '',
        kind: kindFor(input.kind),
        title: input.title,
        description: input.description ?? '',
        condition: input.condition ?? '',
        position: 0,
        reference: ''
      })
    );
    return res.mutation?.changedNodes[0]?.id ?? '';
  }

  async updateNode(nodeId: string, patch: UpdateNodeInput): Promise<void> {
    const updateMask: string[] = [];
    const body: Record<string, unknown> = {
      meta: { author: AUTHOR, idempotencyKey: idemKey() },
      nodeId
    };
    if (patch.title !== undefined) {
      updateMask.push('title');
      body.title = patch.title;
    }
    if (patch.description !== undefined) {
      updateMask.push('description');
      body.description = patch.description;
    }
    if (patch.condition !== undefined) {
      updateMask.push('condition');
      body.condition = patch.condition;
    }
    if (patch.reference !== undefined) {
      updateMask.push('reference');
      body.reference = patch.reference;
    }
    if (updateMask.length === 0) return;
    await this.client.updateNode(create(UpdateNodeRequestSchema, { ...body, updateMask }));
  }

  async deleteNode(nodeId: string): Promise<void> {
    await this.client.deleteNode(
      create(DeleteNodeRequestSchema, {
        meta: { author: AUTHOR, idempotencyKey: idemKey() },
        nodeId,
        failIfReferenced: false
      })
    );
  }

  async addDependency(blockerId: string, blockedId: string): Promise<void> {
    await this.client.addDependency(
      create(AddDependencyRequestSchema, {
        meta: { author: AUTHOR, idempotencyKey: idemKey() },
        blockerId,
        blockedId
      })
    );
  }

  async removeDependency(blockerId: string, blockedId: string): Promise<void> {
    await this.client.removeDependency(
      create(RemoveDependencyRequestSchema, {
        meta: { author: AUTHOR, idempotencyKey: idemKey() },
        blockerId,
        blockedId
      })
    );
  }

  async undo(projectId: string): Promise<void> {
    await this.client.undo(
      create(UndoRequestSchema, {
        meta: { author: AUTHOR, idempotencyKey: idemKey() },
        projectId,
        seq: 0n
      })
    );
  }
}
