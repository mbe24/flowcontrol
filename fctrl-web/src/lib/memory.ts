import type { FlowStore } from './store';
import type { Dependency, FlowNode, Project, Status, VerifyResult } from './types';

const wp = (id: string, title: string, state: FlowNode['state']): FlowNode => ({
  id,
  projectId: 'prj-travel',
  parentId: null,
  type: 'WORK_PACKAGE',
  title,
  status: 'READY',
  state,
  lastResult: 'none'
});

const task = (
  id: string,
  parentId: string,
  title: string,
  status: Status,
  condition: string,
  description: string,
  lastResult: VerifyResult = 'none',
  lastRun = ''
): FlowNode => ({
  id,
  projectId: 'prj-travel',
  parentId,
  type: 'TASK',
  title,
  status,
  condition,
  description,
  lastResult,
  lastRun
});

const step = (
  id: string,
  parentId: string,
  title: string,
  status: Status,
  condition = ''
): FlowNode => ({
  id,
  projectId: 'prj-travel',
  parentId,
  type: 'STEP',
  title,
  status,
  condition,
  lastResult: 'none'
});

const PROJECTS: Project[] = [
  { id: 'prj-travel', name: 'Travel Webapp', description: 'Booking flow, auth and payments.', createdAt: Date.now() },
  { id: 'prj-beer', name: 'Beer App', description: 'Tasting notes and cellar tracking.', createdAt: Date.now() },
  { id: 'prj-docs', name: 'Developer Docs', description: 'Public API reference.', createdAt: Date.now() }
];

const NODES: FlowNode[] = [
  wp('WP-AUTH', 'Authentication Infrastructure', 'ACTIVE'),
  task('T-1041', 'WP-AUTH', 'Session store on Redis with sliding expiry', 'DONE', 'redis-cli ping',
    'Replaces the in-process session map. Sliding expiry of 30m, hard cap 12h.', 'pass', '3d ago'),
  step('T-1041.1', 'T-1041', 'Provision Redis instance', 'DONE', 'terraform apply'),
  step('T-1041.2', 'T-1041', 'Session serializer', 'DONE', 'pnpm test:session'),
  step('T-1041.3', 'T-1041', 'Cut over behind flag', 'DONE', 'manual ✓'),

  task('T-1042', 'WP-AUTH', 'OAuth2 device-code flow for the CLI', 'READY', 'pnpm test:auth --grep device',
    'The TUI and MCP server both authenticate headlessly. Device-code is the only flow that works without a browser redirect on the machine running fctrl.',
    'stale', '2d ago'),
  step('T-1042.1', 'T-1042', 'Register client credentials in provider', 'DONE', 'manual ✓'),
  step('T-1042.2', 'T-1042', 'Poll token endpoint with backoff', 'DONE', 'curl -sf /device/token'),
  step('T-1042.3', 'T-1042', 'Persist refresh token to OS keyring', 'READY', 'fctrl auth whoami'),
  step('T-1042.4', 'T-1042', 'Handle expired_token + slow_down', 'BLOCKED', 'pnpm test:auth --grep slowdown'),
  step('T-1042.5', 'T-1042', 'Docs: CLI login walkthrough', 'BLOCKED', 'file exists: docs/cli-login.md'),

  task('T-1043', 'WP-AUTH', 'Refresh-token rotation + reuse detection', 'BLOCKED', 'pnpm test:auth --grep rotate',
    'Rotate on every refresh; a replayed token invalidates the whole family.'),
  step('T-1043.1', 'T-1043', 'Token family table', 'BLOCKED'),
  step('T-1043.2', 'T-1043', 'Rotation on refresh', 'BLOCKED'),
  step('T-1043.3', 'T-1043', 'Reuse alarm', 'BLOCKED'),
  step('T-1043.4', 'T-1043', 'Backfill existing tokens', 'BLOCKED'),

  task('T-1044', 'WP-AUTH', 'Rate-limit the token endpoint', 'BLOCKED', 'k6 run load/token.js',
    'Needs the metrics pipeline from Observability before limits can be tuned.'),
  step('T-1044.1', 'T-1044', 'Choose limiter algorithm', 'BLOCKED'),
  step('T-1044.2', 'T-1044', 'Wire to metrics', 'BLOCKED'),
  step('T-1044.3', 'T-1044', 'Load test at 5k rps', 'BLOCKED'),

  task('T-1045', 'WP-AUTH', 'Migrate legacy sessions', 'DEFERRED', 'manual sign-off',
    'Parked until the legacy cohort drops below 2% of DAU.'),
  step('T-1045.1', 'T-1045', 'Cohort report', 'DONE', 'manual ✓'),
  step('T-1045.2', 'T-1045', 'Dual-read shim', 'DEFERRED'),
  step('T-1045.3', 'T-1045', 'Backfill job', 'DEFERRED'),
  step('T-1045.4', 'T-1045', 'Cutover', 'DEFERRED'),

  wp('WP-BOOK', 'Booking Engine', 'ACTIVE'),
  task('T-2010', 'WP-BOOK', 'Availability search across provider adapters', 'READY', 'pnpm test:booking',
    'Fan-out to every enabled adapter, merge and de-duplicate by property id.', 'pass', '20m ago'),
  step('T-2010.1', 'T-2010', 'Adapter interface', 'DONE', 'tsc --noEmit'),
  step('T-2010.2', 'T-2010', 'Fan-out with timeout budget', 'DONE', 'pnpm test:booking --grep fanout'),
  step('T-2010.3', 'T-2010', 'Result de-duplication', 'DONE', 'pnpm test:booking --grep dedupe'),
  step('T-2010.4', 'T-2010', 'Currency normalisation', 'DONE', 'manual ✓'),
  step('T-2010.5', 'T-2010', 'Cache layer', 'READY', 'redis-cli ping'),
  step('T-2010.6', 'T-2010', 'Adapter failure isolation', 'BLOCKED'),
  step('T-2010.7', 'T-2010', 'p95 under 800ms', 'BLOCKED', 'k6 run load/search.js'),

  task('T-2011', 'WP-BOOK', 'Hold-and-confirm two-phase reservation', 'BLOCKED', 'pnpm test:booking --grep hold',
    'Holds expire after 10 minutes; confirm is idempotent per hold id.'),
  step('T-2011.1', 'T-2011', 'Hold table + TTL', 'BLOCKED'),
  step('T-2011.2', 'T-2011', 'Confirm endpoint', 'BLOCKED'),
  step('T-2011.3', 'T-2011', 'Expiry sweeper', 'BLOCKED'),

  task('T-2012', 'WP-BOOK', 'Idempotency keys on the confirm endpoint', 'BLOCKED', 'file exists: docs/idempotency.md',
    'Keys are scoped per authenticated principal, so this waits on the CLI auth flow.', 'fail', '3h ago'),
  step('T-2012.1', 'T-2012', 'Key storage + TTL', 'BLOCKED'),
  step('T-2012.2', 'T-2012', 'Replay response cache', 'BLOCKED'),
  step('T-2012.3', 'T-2012', 'Document the contract', 'BLOCKED'),

  wp('WP-PAY', 'Payments', 'ACTIVE'),
  task('T-3007', 'WP-PAY', 'Stripe webhook signature verification', 'READY', 'pnpm test:pay --grep webhook',
    'Reject unsigned or replayed webhook deliveries.'),
  step('T-3007.1', 'T-3007', 'Verify signature header', 'READY'),
  step('T-3007.2', 'T-3007', 'Replay window of 5m', 'BLOCKED'),
  task('T-3001', 'WP-PAY', 'Currency rounding rules', 'DONE', 'pnpm test:pay --grep round', '', 'pass', '1w ago'),
  task('T-3011', 'WP-PAY', 'Refund reconciliation job', 'BLOCKED', 'pnpm test:pay --grep refund',
    'Cannot reconcile refunds until reservations have a stable lifecycle.'),

  wp('WP-OBS', 'Observability', 'PLANNED'),
  task('T-4002', 'WP-OBS', 'Structured log schema for the Rust core', 'READY', 'cargo test --package fctrl-core log',
    'One event shape for the engine, the web app and the TUI.'),
  task('T-4000', 'WP-OBS', 'OTel collector bootstrap', 'DONE', 'kubectl get pods -l otel', '', 'pass', '2w ago'),

  wp('WP-UI', 'UI Redesign', 'PLANNED'),
  task('T-5001', 'WP-UI', 'Dark-mode token audit', 'DEFERRED', 'manual', ''),

  wp('WP-LEGACY', 'Legacy Import', 'DONE'),
  task('T-9001', 'WP-LEGACY', 'One-off CSV import', 'DONE', 'manual', '', 'pass', '3w ago'),

  { id: 'WP-BEER', projectId: 'prj-beer', parentId: null, type: 'WORK_PACKAGE', title: 'Cellar tracking', status: 'READY', state: 'ACTIVE', lastResult: 'none' },
  { id: 'T-8001', projectId: 'prj-beer', parentId: 'WP-BEER', type: 'TASK', title: 'Bottle inventory model', status: 'READY', condition: 'pnpm test:cellar', lastResult: 'none' },
  { id: 'WP-DOCS', projectId: 'prj-docs', parentId: null, type: 'WORK_PACKAGE', title: 'API reference', status: 'READY', state: 'ACTIVE', lastResult: 'none' },
  { id: 'T-7001', projectId: 'prj-docs', parentId: 'WP-DOCS', type: 'TASK', title: 'Generate from OpenAPI', status: 'BLOCKED', condition: 'make docs', lastResult: 'none' }
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

/** Canned verification outcomes, so `Verify` is deterministic. */
const CANNED: Record<string, VerifyResult> = {
  'T-1042': 'pass',
  'T-2012': 'fail',
  'T-3007': 'pass'
};

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

export class MemoryStore implements FlowStore {
  private nodeList: FlowNode[] = NODES.map((n) => ({ ...n }));
  private depList: Dependency[] = DEPS.map((d) => ({ ...d }));

  async projects(): Promise<Project[]> {
    await sleep(60);
    return PROJECTS.map((p) => ({ ...p }));
  }

  async nodes(projectId: string): Promise<FlowNode[]> {
    await sleep(60);
    return this.nodeList.filter((n) => n.projectId === projectId).map((n) => ({ ...n }));
  }

  async dependencies(projectId: string): Promise<Dependency[]> {
    await sleep(20);
    const ids = new Set(this.nodeList.filter((n) => n.projectId === projectId).map((n) => n.id));
    return this.depList.filter((d) => ids.has(d.blockerId) || ids.has(d.blockedId)).map((d) => ({ ...d }));
  }

  async setStatus(nodeId: string, status: Status): Promise<void> {
    await sleep(40);
    const n = this.nodeList.find((x) => x.id === nodeId);
    if (!n) throw new Error(`node not found: ${nodeId}`);
    n.status = status;
  }

  async verify(nodeId: string): Promise<VerifyResult> {
    await sleep(900);
    const res = CANNED[nodeId] ?? 'pass';
    const n = this.nodeList.find((x) => x.id === nodeId);
    if (n) {
      n.lastResult = res;
      n.lastRun = 'just now';
    }
    return res;
  }
}
