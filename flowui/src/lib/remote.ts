import { create } from '@bufbuild/protobuf';
import { createClient, type Client, type Interceptor } from '@connectrpc/connect';
import { createGrpcWebTransport } from '@connectrpc/connect-web';
import type { FlowStore, NewNode, NodePatch } from './store';
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
  ArchiveProjectRequestSchema,
  AgentResult as PbAgentResult,
  CreateNodeRequestSchema,
  CreateProjectRequestSchema,
  DeclaredStatus as PbDeclared,
  DeleteNodeRequestSchema,
  EffectiveStatus as PbEffective,
  EventKind as PbEventKind,
  FlowService,
  GetSnapshotRequestSchema,
  HumanVerdict as PbVerdict,
  ListProjectsRequestSchema,
  MoveNodeRequestSchema,
  NodeKind as PbKind,
  RemoveDependencyRequestSchema,
  SetStatusRequestSchema,
  SetVerdictRequestSchema,
  UpdateNodeRequestSchema,
  UpdateProjectRequestSchema,
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
  // Explicit override wins (e.g. a team pointing at a shared daemon). Otherwise the
  // SPA is served BY the daemon, so its own origin is the API — same-origin, which
  // is the whole point of the transport design (no mixed content / CORS / certs).
  if (import.meta.env.VITE_SERVER_URL) return import.meta.env.VITE_SERVER_URL;
  if (typeof window !== 'undefined' && window.location?.origin) return window.location.origin;
  return DEFAULT_BASE_URL;
}

/**
 * The daemon serves the SPA and injects its bearer token into index.html as
 * `<meta name="flow-token">`; we read it and authenticate every RPC. In dev
 * (Vite-served, not injected) fall back to VITE_FLOW_TOKEN. Empty → no header
 * (a daemon started without a token).
 */
function flowToken(): string {
  if (typeof document !== 'undefined') {
    const meta = document.querySelector('meta[name="flow-token"]');
    const v = meta?.getAttribute('content');
    if (v) return v;
  }
  return import.meta.env.VITE_FLOW_TOKEN ?? '';
}

function bearer(token: string): Interceptor {
  return (next) => (req) => {
    req.header.set('Authorization', `Bearer ${token}`);
    return next(req);
  };
}

function idemKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function meta() {
  return { author: AUTHOR, idempotencyKey: idemKey() };
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

function toDeclared(status: Status): PbDeclared {
  switch (status) {
    case 'DONE':
      return PbDeclared.DONE;
    case 'DEFERRED':
      return PbDeclared.DEFERRED;
    default:
      return PbDeclared.OPEN;
  }
}

function toVerdict(verdict: HumanVerdict): PbVerdict {
  switch (verdict) {
    case 'accepted':
      return PbVerdict.ACCEPTED;
    case 'rejected':
      return PbVerdict.REJECTED;
    default:
      return PbVerdict.UNSPECIFIED;
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

function wpStateFor(s: WPState): PbWpState {
  switch (s) {
    case 'PLANNED':
      return PbWpState.PLANNED;
    case 'ACTIVE':
      return PbWpState.ACTIVE;
    case 'DONE':
      return PbWpState.DONE;
    case 'ARCHIVED':
      return PbWpState.ARCHIVED;
  }
}

function verificationOf(v: PbNode['verification']): Verification | undefined {
  if (!v) return undefined;
  const agent: AgentResult = v.stale ? 'stale' : v.agentResult === PbAgentResult.PASS ? 'pass' : v.agentResult === PbAgentResult.FAIL ? 'fail' : 'none';
  let human: HumanVerdict = 'none';
  if (v.humanVerdict === PbVerdict.ACCEPTED) human = 'accepted';
  else if (v.humanVerdict === PbVerdict.REJECTED) human = 'rejected';
  if (agent === 'none' && human === 'none') return undefined;
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
    createdAt: Number(p.createdAt),
    archived: Number(p.archivedAt) !== 0
  };
}

export function mapNode(n: PbNode): FlowNode {
  const node: FlowNode = {
    id: n.id,
    projectId: n.projectId,
    parentId: n.parentId || null,
    type: kindOf(n.kind),
    title: n.title,
    description: n.description ? n.description.split(/\n{2,}/).map((s) => s.trim()).filter(Boolean) : [],
    status: statusOf(n.status)
  };
  if (n.condition) node.condition = n.condition;
  if (n.note) node.note = n.note;
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
  | 'moveNode'
  | 'createProject'
  | 'updateProject'
  | 'archiveProject'
>;

/**
 * FlowStore backed by the FlowControl core over grpc-web. Every method maps to a
 * FlowService RPC — project lifecycle (create/update/archive) and moveNode
 * included; `note` round-trips on read and update.
 */
export class RemoteStore implements FlowStore {
  private client: FlowServiceShim;

  constructor(client?: FlowServiceShim) {
    const token = flowToken();
    this.client =
      client ??
      createClient(
        FlowService,
        createGrpcWebTransport({
          baseUrl: baseUrl(),
          useBinaryFormat: true,
          interceptors: token ? [bearer(token)] : []
        })
      );
  }

  private async snapshot(projectId: string) {
    return this.client.getSnapshot(create(GetSnapshotRequestSchema, { projectId }));
  }

  async projects(): Promise<Project[]> {
    const res = await this.client.listProjects(create(ListProjectsRequestSchema, { includeArchived: true }));
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
    await this.client.setStatus(create(SetStatusRequestSchema, { meta: meta(), nodeId, declaredStatus: toDeclared(status) }));
  }

  async setVerdict(nodeId: string, verdict: HumanVerdict): Promise<void> {
    await this.client.setVerdict(create(SetVerdictRequestSchema, { meta: meta(), nodeId, verdict: toVerdict(verdict) }));
  }

  /** Work-package lifecycle maps to UpdateNode's wp_state field. */
  async setWpState(nodeId: string, state: WPState): Promise<void> {
    await this.client.updateNode(
      create(UpdateNodeRequestSchema, { meta: meta(), nodeId, updateMask: ['wp_state'], wpState: wpStateFor(state) })
    );
  }

  async addComment(nodeId: string, text: string): Promise<void> {
    await this.client.addComment(create(AddCommentRequestSchema, { meta: meta(), nodeId, text }));
  }

  async createNode(input: NewNode): Promise<string> {
    const res = await this.client.createNode(
      create(CreateNodeRequestSchema, {
        meta: meta(),
        projectId: input.projectId,
        parentId: input.parentId ?? '',
        kind: kindFor(input.type),
        title: input.title,
        description: (input.description ?? []).join('\n\n'),
        condition: input.condition ?? '',
        note: input.note ?? '',
        position: 0,
        reference: ''
      })
    );
    return res.mutation?.changedNodes[0]?.id ?? '';
  }

  async updateNode(nodeId: string, patch: NodePatch): Promise<void> {
    const updateMask: string[] = [];
    const body: Record<string, unknown> = { meta: meta(), nodeId };
    if (patch.title !== undefined) {
      updateMask.push('title');
      body.title = patch.title;
    }
    if (patch.description !== undefined) {
      updateMask.push('description');
      body.description = patch.description.join('\n\n');
    }
    if (patch.condition !== undefined) {
      updateMask.push('condition');
      body.condition = patch.condition;
    }
    if (patch.note !== undefined) {
      updateMask.push('note');
      body.note = patch.note;
    }
    if (updateMask.length === 0) return;
    await this.client.updateNode(create(UpdateNodeRequestSchema, { ...body, updateMask }));
  }

  async deleteNode(nodeId: string): Promise<void> {
    await this.client.deleteNode(create(DeleteNodeRequestSchema, { meta: meta(), nodeId, failIfReferenced: false }));
  }

  async moveNode(nodeId: string, newParentId: string, newType: NodeType): Promise<void> {
    await this.client.moveNode(
      create(MoveNodeRequestSchema, { meta: meta(), nodeId, parentId: newParentId, kind: kindFor(newType) })
    );
  }

  async addDependency(blockerId: string, blockedId: string): Promise<void> {
    await this.client.addDependency(create(AddDependencyRequestSchema, { meta: meta(), blockerId, blockedId }));
  }

  async removeDependency(blockerId: string, blockedId: string): Promise<void> {
    await this.client.removeDependency(create(RemoveDependencyRequestSchema, { meta: meta(), blockerId, blockedId }));
  }

  async createProject(name: string, description: string, seedWorkPackage: boolean): Promise<string> {
    const res = await this.client.createProject(create(CreateProjectRequestSchema, { meta: meta(), name, description }));
    const id = res.project?.id ?? '';
    // Seeding is client-side composition: create the project, then its first WP.
    if (id && seedWorkPackage) {
      await this.createNode({ projectId: id, parentId: null, type: 'WORK_PACKAGE', title: name });
    }
    return id;
  }

  async updateProject(projectId: string, patch: { name?: string; description?: string }): Promise<void> {
    const updateMask: string[] = [];
    const body: Record<string, unknown> = { meta: meta(), projectId };
    if (patch.name !== undefined) {
      updateMask.push('name');
      body.name = patch.name;
    }
    if (patch.description !== undefined) {
      updateMask.push('description');
      body.description = patch.description;
    }
    if (updateMask.length === 0) return;
    await this.client.updateProject(create(UpdateProjectRequestSchema, { ...body, updateMask }));
  }

  async archiveProject(projectId: string, archived: boolean): Promise<void> {
    await this.client.archiveProject(create(ArchiveProjectRequestSchema, { meta: meta(), projectId, archived }));
  }
}