import type { CreateNodeInput, FlowStore, UpdateNodeInput } from './store';
import type {
  ActivityEntry,
  AgentResult,
  Dependency,
  FlowNode,
  HumanVerdict,
  Project,
  Status,
  Verification
} from './types';
import { NO_VERIFICATION } from './types';

const ver = (agent: AgentResult, agentName = '', agentWhen = '', human: HumanVerdict = 'none', humanWhen = ''): Verification => ({
  agent,
  agentName,
  agentWhen,
  human,
  humanWhen
});

const wp = (id: string, title: string, state: FlowNode['state']): FlowNode => ({
  id,
  projectId: 'prj-travel',
  parentId: null,
  type: 'WORK_PACKAGE',
  title,
  description: [],
  status: 'READY',
  state
});

const task = (
  id: string,
  parentId: string,
  title: string,
  status: Status,
  condition: string,
  description: string[],
  verification: Verification = NO_VERIFICATION
): FlowNode => ({
  id,
  projectId: 'prj-travel',
  parentId,
  type: 'TASK',
  title,
  description,
  status,
  condition,
  verification
});

const step = (id: string, parentId: string, title: string, status: Status, condition = '', note = ''): FlowNode => ({
  id,
  projectId: 'prj-travel',
  parentId,
  type: 'STEP',
  title,
  description: [],
  status,
  condition,
  note
});

const PROJECTS: Project[] = [
  { id: 'prj-travel', name: 'Travel Webapp', description: 'Booking flow, auth and payments.', createdAt: Date.now() },
  { id: 'prj-beer', name: 'Beer App', description: 'Tasting notes and cellar tracking.', createdAt: Date.now() },
  { id: 'prj-docs', name: 'Developer Docs', description: 'Public API reference.', createdAt: Date.now() }
];

const NODES: FlowNode[] = [
  wp('WP-AUTH', 'Authentication Infrastructure', 'ACTIVE'),

  task('T-1041', 'WP-AUTH', 'Session store on Redis with sliding expiry', 'DONE', 'redis-cli ping',
    ['Replaces the in-process session map so sessions survive a deploy.',
     'Sliding expiry of 30 minutes with a hard cap of 12 hours. The cap matters more than the slide — without it a tab left open for a week keeps a session alive forever.'],
    ver('pass', 'claude-code', '3d ago')),
  step('T-1041.1', 'T-1041', 'Provision Redis instance', 'DONE', 'terraform apply',
    'Single node with AOF persistence. A cluster is overkill until sessions outgrow one box.'),
  step('T-1041.2', 'T-1041', 'Session serializer', 'DONE', 'pnpm test:session',
    'MessagePack rather than JSON — roughly 40% smaller for our shape, and the decode cost is in the noise.'),
  step('T-1041.3', 'T-1041', 'Cut over behind flag', 'DONE', 'manual ✓', ''),

  task('T-1042', 'WP-AUTH', 'OAuth2 device-code flow for the CLI', 'READY', 'pnpm test:auth --grep device',
    ['The TUI and MCP server both authenticate headlessly. Device-code is the only flow that works without a browser redirect on the machine running fctrl.',
     'The provider caps polling at one request every five seconds and returns slow_down when exceeded, so the client needs real backoff rather than a fixed interval.',
     'Refresh tokens land in the OS keyring — Keychain, libsecret, or Credential Manager — never on disk in plaintext.'],
    ver('pass', 'claude-code', '2d ago')),
  step('T-1042.1', 'T-1042', 'Register client credentials in provider', 'DONE', 'manual ✓',
    'Done in the provider console; the client id lives in 1Password under "fctrl oauth".'),
  step('T-1042.2', 'T-1042', 'Poll token endpoint with backoff', 'DONE', 'curl -sf /device/token',
    'Exponential from 5s with a 30s ceiling, honouring the slow_down hint by adding 5s to the interval each time it appears.'),
  step('T-1042.3', 'T-1042', 'Persist refresh token to OS keyring', 'READY', 'fctrl auth whoami',
    'Keychain on macOS, libsecret on Linux, Credential Manager on Windows. Headless Linux has no libsecret — fall back to a mode-0600 file and warn loudly.'),
  step('T-1042.4', 'T-1042', 'Handle expired_token + slow_down', 'BLOCKED', 'pnpm test:auth --grep slowdown',
    'Blocked on the error taxonomy being settled in T-1043.'),
  step('T-1042.5', 'T-1042', 'Docs: CLI login walkthrough', 'BLOCKED', 'file exists: docs/cli-login.md', ''),

  task('T-1043', 'WP-AUTH', 'Refresh-token rotation + reuse detection', 'BLOCKED', 'pnpm test:auth --grep rotate',
    ['Rotate on every refresh; a replayed token invalidates the whole family.',
     'Reuse almost always means theft. Killing the family logs the legitimate user out too, which is the correct trade.'],
    NO_VERIFICATION),
  step('T-1043.1', 'T-1043', 'Token family table', 'BLOCKED', '', 'One row per family, not per token. Tokens are derived.'),
  step('T-1043.2', 'T-1043', 'Rotation on refresh', 'BLOCKED', '', ''),
  step('T-1043.3', 'T-1043', 'Reuse alarm', 'BLOCKED', '', 'Page on it. A reuse event is a live incident, not a metric.'),
  step('T-1043.4', 'T-1043', 'Backfill existing tokens', 'BLOCKED', '', ''),

  task('T-1044', 'WP-AUTH', 'Rate-limit the token endpoint', 'BLOCKED', 'k6 run load/token.js',
    ['Needs the metrics pipeline from Observability before limits can be tuned to anything but a guess.'],
    NO_VERIFICATION),
  step('T-1044.1', 'T-1044', 'Choose limiter algorithm', 'BLOCKED', '', 'Sliding window over token bucket — bursty CLI logins are legitimate.'),
  step('T-1044.2', 'T-1044', 'Wire to metrics', 'BLOCKED', '', ''),
  step('T-1044.3', 'T-1044', 'Load test at 5k rps', 'BLOCKED', '', ''),

  task('T-1045', 'WP-AUTH', 'Migrate legacy sessions', 'DEFERRED', 'manual sign-off',
    ['Parked until the legacy cohort drops below 2% of DAU. Currently 6.4% and falling about half a point a month.'],
    ver('stale', 'claude-code', '3w ago')),
  step('T-1045.1', 'T-1045', 'Cohort report', 'DONE', 'manual ✓', 'Refreshed weekly into the Observability dashboard.'),
  step('T-1045.2', 'T-1045', 'Dual-read shim', 'DEFERRED', '', ''),
  step('T-1045.3', 'T-1045', 'Backfill job', 'DEFERRED', '', ''),
  step('T-1045.4', 'T-1045', 'Cutover', 'DEFERRED', '', ''),

  task('T-1046', 'WP-AUTH', 'Audit-log every token issuance', 'READY', 'pnpm test:auth --grep audit',
    ['Compliance wants issuance, refresh and revocation events retained for a year.'],
    ver('fail', 'claude-code', '4h ago')),
  step('T-1046.1', 'T-1046', 'Event schema', 'READY', '', ''),
  step('T-1046.2', 'T-1046', 'Write path', 'BLOCKED', '', ''),
  step('T-1046.3', 'T-1046', 'Retention policy', 'BLOCKED', '', ''),

  wp('WP-BOOK', 'Booking Engine', 'ACTIVE'),
  task('T-2010', 'WP-BOOK', 'Availability search across provider adapters', 'READY', 'pnpm test:booking',
    ['Fan-out to every enabled adapter, merge and de-duplicate by property id.',
     'A slow adapter must not hold the whole response. Each gets a 600ms budget and anything late is dropped from this query.'],
    ver('pass', 'claude-code', '20m ago')),
  step('T-2010.1', 'T-2010', 'Adapter interface', 'DONE', 'tsc --noEmit', ''),
  step('T-2010.2', 'T-2010', 'Fan-out with timeout budget', 'DONE', 'pnpm test:booking --grep fanout', 'Promise.allSettled with a per-adapter AbortController.'),
  step('T-2010.3', 'T-2010', 'Result de-duplication', 'DONE', 'pnpm test:booking --grep dedupe', 'Match on provider property id first, then on a normalised name + coordinate pair within 50m.'),
  step('T-2010.4', 'T-2010', 'Currency normalisation', 'DONE', 'manual ✓', ''),
  step('T-2010.5', 'T-2010', 'Cache layer', 'READY', 'redis-cli ping', 'Five minute TTL keyed on the normalised query.'),
  step('T-2010.6', 'T-2010', 'Adapter failure isolation', 'BLOCKED', '', ''),
  step('T-2010.7', 'T-2010', 'p95 under 800ms', 'BLOCKED', 'k6 run load/search.js', ''),

  task('T-2011', 'WP-BOOK', 'Hold-and-confirm two-phase reservation', 'BLOCKED', 'pnpm test:booking --grep hold',
    ['Holds expire after 10 minutes; confirm is idempotent per hold id.'], NO_VERIFICATION),
  step('T-2011.1', 'T-2011', 'Hold table + TTL', 'BLOCKED', '', ''),
  step('T-2011.2', 'T-2011', 'Confirm endpoint', 'BLOCKED', '', ''),
  step('T-2011.3', 'T-2011', 'Expiry sweeper', 'BLOCKED', '', 'Runs every 30s. A missed sweep is harmless; a double-release is not.'),

  task('T-2012', 'WP-BOOK', 'Idempotency keys on the confirm endpoint', 'BLOCKED', 'file exists: docs/idempotency.md',
    ['Keys are scoped per authenticated principal, so this waits on the CLI auth flow.'],
    ver('fail', 'claude-code', '3h ago')),
  step('T-2012.1', 'T-2012', 'Key storage + TTL', 'BLOCKED', '', ''),
  step('T-2012.2', 'T-2012', 'Replay response cache', 'BLOCKED', '', ''),
  step('T-2012.3', 'T-2012', 'Document the contract', 'BLOCKED', '', ''),

  wp('WP-PAY', 'Payments', 'ACTIVE'),
  task('T-3007', 'WP-PAY', 'Stripe webhook signature verification', 'READY', 'pnpm test:pay --grep webhook',
    ['Reject unsigned or replayed webhook deliveries.'], NO_VERIFICATION),
  step('T-3007.1', 'T-3007', 'Verify signature header', 'READY', '', ''),
  step('T-3007.2', 'T-3007', 'Replay window of 5m', 'BLOCKED', '', ''),
  task('T-3001', 'WP-PAY', 'Currency rounding rules', 'DONE', 'pnpm test:pay --grep round',
    ['Banker\u2019s rounding at the line level, not the total.'], ver('pass', 'claude-code', '1w ago')),
  task('T-3011', 'WP-PAY', 'Refund reconciliation job', 'BLOCKED', 'pnpm test:pay --grep refund',
    ['Cannot reconcile refunds until reservations have a stable lifecycle.'], NO_VERIFICATION),

  wp('WP-OBS', 'Observability', 'PLANNED'),
  task('T-4002', 'WP-OBS', 'Structured log schema for the Rust core', 'READY', 'cargo test --package fctrl-core log',
    ['One event shape for the engine, the web app and the TUI.'], NO_VERIFICATION),
  task('T-4000', 'WP-OBS', 'OTel collector bootstrap', 'DONE', 'kubectl get pods -l otel', [], ver('pass', 'claude-code', '2w ago')),

  wp('WP-UI', 'UI Redesign', 'PLANNED'),
  task('T-5001', 'WP-UI', 'Dark-mode token audit', 'DEFERRED', 'manual', [], NO_VERIFICATION),

  wp('WP-LEGACY', 'Legacy Import', 'DONE'),
  task('T-9001', 'WP-LEGACY', 'One-off CSV import', 'DONE', 'manual', [], ver('pass', 'you', '3w ago', 'accepted', '3w ago')),

  { id: 'WP-BEER', projectId: 'prj-beer', parentId: null, type: 'WORK_PACKAGE', title: 'Cellar tracking', description: [], status: 'READY', state: 'ACTIVE' },
  { id: 'T-8001', projectId: 'prj-beer', parentId: 'WP-BEER', type: 'TASK', title: 'Bottle inventory model', description: ['Track what is in the cellar and when it should be drunk.'], status: 'READY', condition: 'pnpm test:cellar', verification: NO_VERIFICATION },
  { id: 'WP-DOCS', projectId: 'prj-docs', parentId: null, type: 'WORK_PACKAGE', title: 'API reference', description: [], status: 'READY', state: 'ACTIVE' },
  { id: 'T-7001', projectId: 'prj-docs', parentId: 'WP-DOCS', type: 'TASK', title: 'Generate from OpenAPI', description: [], status: 'BLOCKED', condition: 'make docs', verification: NO_VERIFICATION }
];

const DEPS: Dependency[] = [
  { blockerId: 'T-1042', blockedId: 'T-1043' },
  { blockerId: 'T-1043', blockedId: 'T-1044' },
  { blockerId: 'WP-OBS', blockedId: 'T-1044' },
  { blockerId: 'T-2010', blockedId: 'T-2011' },
  { blockerId: 'T-2011', blockedId: 'T-2012' },
  { blockerId: 'T-1042', blockedId: 'T-2012' },
  { blockerId: 'WP-BOOK', blockedId: 'T-3011' },
  { blockerId: 'T-3007', blockedId: 'T-3011' }
];

const ACTIVITY: ActivityEntry[] = [
  { id: 'a1', nodeId: 'T-1042', kind: 'verify', author: 'claude-code', when: '2d ago', text: 'Reported condition passed' },
  { id: 'a2', nodeId: 'T-1042', kind: 'status', author: 'you', when: '2d ago', text: 'BLOCKED → READY, the keyring work can start' },
  { id: 'a3', nodeId: 'T-1042', kind: 'edit', author: 'claude-code', when: '3d ago', text: 'Marked step “Poll token endpoint with backoff” done' },
  { id: 'a4', nodeId: 'T-1042', kind: 'comment', author: 'you', when: '4d ago', text: 'Split the polling work out of T-1041 — it was doing too much.' },
  { id: 'a5', nodeId: 'T-1046', kind: 'verify', author: 'claude-code', when: '4h ago', text: 'Reported condition failed: 2 assertions in audit.spec.ts' },
  { id: 'a6', nodeId: 'T-1046', kind: 'comment', author: 'claude-code', when: '4h ago', text: 'The failures are in the retention assertions, not the write path. The write path is sound.' },
  { id: 'a7', nodeId: 'T-2010', kind: 'verify', author: 'claude-code', when: '20m ago', text: 'Reported condition passed' },
  { id: 'a8', nodeId: 'T-2010', kind: 'edit', author: 'claude-code', when: '1h ago', text: 'Added step “Adapter failure isolation”' },
  { id: 'a9', nodeId: 'T-2012', kind: 'verify', author: 'claude-code', when: '3h ago', text: 'Reported condition failed: docs/idempotency.md not found' },
  { id: 'a10', nodeId: 'T-1041', kind: 'status', author: 'you', when: '3d ago', text: 'READY → DONE' }
];

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

export class MemoryStore implements FlowStore {
  private nodeList: FlowNode[] = NODES.map((n) => ({ ...n, verification: n.verification ? { ...n.verification } : undefined }));
  private depList: Dependency[] = DEPS.map((d) => ({ ...d }));
  private activityList: ActivityEntry[] = ACTIVITY.map((a) => ({ ...a }));
  private seq = 100;
  /** Ids this session created, in order; backs the fixture's undo. */
  private createdStack: string[] = [];

  async projects(): Promise<Project[]> {
    await sleep(60);
    return PROJECTS.map((p) => ({ ...p }));
  }

  async nodes(projectId: string): Promise<FlowNode[]> {
    await sleep(60);
    return this.nodeList
      .filter((n) => n.projectId === projectId)
      .map((n) => ({ ...n, verification: n.verification ? { ...n.verification } : undefined }));
  }

  async dependencies(projectId: string): Promise<Dependency[]> {
    await sleep(20);
    const ids = new Set(this.nodeList.filter((n) => n.projectId === projectId).map((n) => n.id));
    return this.depList.filter((d) => ids.has(d.blockerId) || ids.has(d.blockedId)).map((d) => ({ ...d }));
  }

  async activity(projectId: string): Promise<ActivityEntry[]> {
    await sleep(20);
    const ids = new Set(this.nodeList.filter((n) => n.projectId === projectId).map((n) => n.id));
    return this.activityList.filter((a) => ids.has(a.nodeId)).map((a) => ({ ...a }));
  }

  async setStatus(nodeId: string, status: Status): Promise<void> {
    await sleep(40);
    const n = this.nodeList.find((x) => x.id === nodeId);
    if (!n) throw new Error(`node not found: ${nodeId}`);
    const prev = n.status;
    n.status = status;
    this.push(nodeId, 'status', `${prev} → ${status}`);
  }

  async setVerdict(nodeId: string, verdict: HumanVerdict): Promise<void> {
    await sleep(40);
    const n = this.nodeList.find((x) => x.id === nodeId);
    if (!n || !n.verification) return;
    n.verification = { ...n.verification, human: verdict, humanWhen: verdict === 'none' ? '' : 'just now' };
    if (verdict === 'accepted') this.push(nodeId, 'verify', 'Accepted the condition as verified');
    else if (verdict === 'rejected') this.push(nodeId, 'verify', 'Rejected the condition');
    else this.push(nodeId, 'verify', 'Cleared the verification override');
  }

  async addComment(nodeId: string, text: string): Promise<void> {
    await sleep(40);
    this.push(nodeId, 'comment', text);
  }

  async createNode(input: CreateNodeInput): Promise<string> {
    await sleep(40);
    const node: FlowNode = {
      id: `node-${this.seq++}`,
      projectId: input.projectId,
      parentId: input.parentId,
      type: input.kind,
      title: input.title,
      description: input.description ? [input.description] : [],
      status: 'READY',
      condition: input.condition || undefined,
      state: input.kind === 'WORK_PACKAGE' ? 'PLANNED' : undefined,
      verification: NO_VERIFICATION
    };
    this.nodeList = [...this.nodeList, node];
    this.createdStack.push(node.id);
    this.push(node.id, 'edit', `created ${node.id}`);
    return node.id;
  }

  async deleteNode(nodeId: string): Promise<void> {
    await sleep(40);
    this.nodeList = this.nodeList.filter((n) => n.id !== nodeId && n.parentId !== nodeId);
    this.depList = this.depList.filter((d) => d.blockerId !== nodeId && d.blockedId !== nodeId);
    this.push(nodeId, 'edit', `deleted ${nodeId}`);
    this.createdStack = this.createdStack.filter((id) => id !== nodeId);
  }

  async updateNode(nodeId: string, patch: UpdateNodeInput): Promise<void> {
    await sleep(40);
    const n = this.nodeList.find((x) => x.id === nodeId);
    if (!n) throw new Error(`node not found: ${nodeId}`);
    if (patch.title !== undefined) n.title = patch.title;
    if (patch.description !== undefined) n.description = patch.description ? [patch.description] : [];
    if (patch.condition !== undefined) n.condition = patch.condition;
    if (patch.reference !== undefined) (n as { reference?: string }).reference = patch.reference;
    this.push(nodeId, 'edit', `updated ${nodeId}`);
  }

  async addDependency(blockerId: string, blockedId: string): Promise<void> {
    await sleep(40);
    if (!this.depList.some((d) => d.blockerId === blockerId && d.blockedId === blockedId)) {
      this.depList = [...this.depList, { blockerId, blockedId }];
    }
    this.push(blockedId, 'edit', `${blockerId} blocks ${blockedId}`);
  }

  async removeDependency(blockerId: string, blockedId: string): Promise<void> {
    await sleep(40);
    this.depList = this.depList.filter((d) => !(d.blockerId === blockerId && d.blockedId === blockedId));
    this.push(blockedId, 'edit', `${blockerId} no longer blocks ${blockedId}`);
  }

  async undo(projectId: string): Promise<void> {
    await sleep(40);
    const id = this.createdStack.pop();
    if (!id) return;
    this.nodeList = this.nodeList.filter((n) => n.id !== id && n.parentId !== id);
    this.depList = this.depList.filter((d) => d.blockerId !== id && d.blockedId !== id);
    this.push(id, 'edit', `undid creation of ${id}`);
  }

  private push(nodeId: string, kind: ActivityEntry['kind'], text: string) {
    this.activityList = [
      { id: `a${this.seq++}`, nodeId, kind, author: 'you', when: 'just now', text },
      ...this.activityList
    ];
  }
}
