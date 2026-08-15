import { Code, ConnectError } from '@connectrpc/connect';

import { createBrowserClient } from './browser/client';
import { RemoteStore } from './remote';
import type { FlowStore, NewNode, NodePatch } from './store';
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

// The demo/offline build runs the REAL flowcore engine in-browser (wasm over a
// durable OPFS SQLite, in a Worker); the networked build talks to flowd/flowd.js
// over grpc-web. Both are the same RemoteStore over a FlowService client — only
// the transport differs.
export let store: FlowStore = import.meta.env.VITE_DEMO
  ? new RemoteStore(createBrowserClient())
  : new RemoteStore();

/** Swap the store (tests inject a mocked FlowStore). */
export function setStore(s: FlowStore) {
  store = s;
}

export type ViewName = 'table' | 'lanes' | 'graph';
/** Desktop: how much room the detail surface takes. Persisted. */
export type PanelMode = 'peek' | 'expanded';
/** Mobile: where the sheet is resting. */
export type SheetMode = 'closed' | 'peek' | 'full';

/** Every modal in the app is one of these. */
/** Which section of the detail panel to reveal when it opens. */
export type FocusTarget =
  | { section: 'title' | 'deps' | 'steps'; nodeId?: string; field?: 'title' | 'condition' | 'note' }
  | null;

export type Dialog =
  | { kind: 'create'; nodeType: NodeType; parentId: string | null; title: string }
  | { kind: 'delete'; nodeId: string }
  | { kind: 'move'; nodeId: string; to: NodeType }
  | { kind: 'newProject' }
  | { kind: 'editProject'; projectId: string }
  | null;

const PANEL_KEY = 'fctrl.panelMode';
const THEME_KEY = 'fctrl.theme';

/** Below this the side panel becomes a bottom sheet. */
export const MOBILE_BP = 860;

function stored<T extends string>(key: string, fallback: T): T {
  try {
    return (localStorage.getItem(key) as T) || fallback;
  } catch {
    return fallback;
  }
}

function remember(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* private mode — the preference just won't persist */
  }
}

export const app = $state({
  loading: true,
  error: '' as string,

  /**
   * Connection to the daemon. On a transport drop we keep the last snapshot and
   * flip to 'disconnected' + a reconnect poll — never a blank page (see
   * design.daemon-lifecycle.md). Always 'connected' for the in-browser store.
   */
  connection: 'connected' as 'connected' | 'disconnected',
  /** Unix ms of the last successful sync, for the "last synced …" banner. */
  lastSyncedAt: 0,

  projects: [] as Project[],
  projectId: 'prj-travel',
  nodes: [] as FlowNode[],
  deps: [] as Dependency[],
  activity: [] as ActivityEntry[],

  view: 'table' as ViewName,
  theme: 'dark' as 'dark' | 'light',
  selectedId: 'T-1042' as string | null,

  /** Viewport width, tracked so the detail surface can switch presentation. */
  width: 1440,

  panelMode: 'peek' as PanelMode,
  sheet: 'closed' as SheetMode,
  dragging: false,

  paletteOpen: false,
  paletteQuery: '',
  editMode: false,
  showArchived: false,

  // ── filters ───────────────────────────────────────────────────────────────
  statusFilter: [] as Status[],
  /** Work-package ids; empty means all. */
  wpFilter: [] as string[],
  /** Verification buckets: 'verified' | 'failed' | 'stale' | 'none'. */
  verFilter: [] as string[],
  showSteps: false,
  filterOpen: false,

  /** Lanes view on mobile shows one lane at a time. */
  laneIndex: 0,

  // ── graph ─────────────────────────────────────────────────────────────────
  zoom: 1,
  /** Live edge being dragged between ports, in canvas coordinates. */
  pendingEdge: null as { fromId: string; x: number; y: number } | null,

  expandedWp: {} as Record<string, boolean>,
  expandedTask: { 'T-1042': true } as Record<string, boolean>,
  expandedStep: {} as Record<string, boolean>,

  /**
   * Why the detail panel was opened, so it can scroll to and focus the right
   * section. Every NodeMenu item used to just set selectedId, which made
   * "Add dependency" look broken — the picker was there, below the fold.
   * DetailBody consumes this once on mount and clears it.
   */
  focusTarget: null as FocusTarget,

  /** Set when a human override would contradict an agent failure. */
  confirmOverride: null as string | null,
  dialog: null as Dialog,
  projectMenuOpen: false,
  nodeMenuFor: null as string | null,
  /** Viewport coords the node menu opens at. */
  menuAt: { x: 0, y: 0 },
  draftComment: '',
  flash: ''
});

export const isMobile = () => app.width < MOBILE_BP;

// ── connection / degrade-and-reconnect ───────────────────────────────────────

function synced() {
  app.connection = 'connected';
  app.lastSyncedAt = Date.now();
  app.error = '';
}

/** A transport drop (daemon gone) vs a real domain error — only the former degrades. */
function isTransportDown(e: unknown): boolean {
  return e instanceof ConnectError && (e.code === Code.Unavailable || e.code === Code.DeadlineExceeded);
}

let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

/** Flip to disconnected and start (once) a poll that retries until the daemon answers. */
function scheduleReconnect() {
  app.connection = 'disconnected';
  if (reconnectTimer) return;
  const tick = async () => {
    reconnectTimer = null;
    if (!(await retryNow())) reconnectTimer = setTimeout(tick, 2000);
  };
  reconnectTimer = setTimeout(tick, 2000);
}

/** One reconnect attempt: re-fetch the current view. Exposed for the banner + tests. */
export async function retryNow(): Promise<boolean> {
  try {
    app.projects = await store.projects();
    await refreshCore();
    synced();
    return true;
  } catch (e) {
    if (isTransportDown(e)) return false;
    app.error = String(e);
    return false;
  }
}

export async function boot() {
  app.theme = stored<'dark' | 'light'>(THEME_KEY, 'dark');
  app.panelMode = stored<PanelMode>(PANEL_KEY, 'peek');
  document.documentElement.dataset.theme = app.theme;
  app.width = window.innerWidth;
  window.addEventListener('resize', () => {
    app.width = window.innerWidth;
  });
  try {
    app.projects = await store.projects();
    await load(app.projectId);
  } catch (e) {
    if (isTransportDown(e)) scheduleReconnect();
    else app.error = String(e);
  }
}

export async function load(projectId: string) {
  app.loading = true;
  app.projectId = projectId;
  app.projectMenuOpen = false;
  try {
    await refreshCore();
    for (const n of app.nodes) {
      if (n.type === 'WORK_PACKAGE' && n.state === 'ACTIVE') app.expandedWp[n.id] = true;
    }
    if (app.selectedId && !app.nodes.some((n) => n.id === app.selectedId)) {
      app.selectedId = app.nodes.find((n) => n.type === 'TASK')?.id ?? null;
    }
    synced();
  } catch (e) {
    if (isTransportDown(e)) scheduleReconnect(); // keep the last snapshot on screen
    else app.error = String(e);
  } finally {
    app.loading = false;
  }
}

/** Fetch the current project's graph into `app`. Throws on failure (caller degrades). */
async function refreshCore() {
  const [nodes, deps, activity] = await Promise.all([
    store.nodes(app.projectId),
    store.dependencies(app.projectId),
    store.activity(app.projectId)
  ]);
  app.nodes = nodes;
  app.deps = deps;
  app.activity = activity;
}

async function refresh() {
  try {
    await refreshCore();
    synced();
  } catch (e) {
    if (isTransportDown(e)) scheduleReconnect(); // keep last snapshot, poll to recover
    else throw e;
  }
}

let lastChange: { id: string; prev: Status } | null = null;

export async function setStatus(id: string, status: Status) {
  const node = app.nodes.find((n) => n.id === id);
  if (!node || node.status === status) return;
  lastChange = { id, prev: node.status };
  await store.setStatus(id, status);
  await refresh();
  app.flash = `${id} → ${status} · the engine owns the cascade`;
}

export async function undo() {
  if (!lastChange) return;
  const { id, prev } = lastChange;
  lastChange = null;
  await store.setStatus(id, prev);
  await refresh();
  app.flash = `undid ${id}`;
}

/**
 * Ticking the box when the agent reported a failure needs confirmation —
 * everything else applies straight away.
 */
export async function toggleVerified(id: string) {
  const node = app.nodes.find((n) => n.id === id);
  if (!node?.verification) return;
  const isAccepted = node.verification.human === 'accepted';
  if (!isAccepted && node.verification.agent === 'fail') {
    app.confirmOverride = id;
    return;
  }
  await store.setVerdict(id, isAccepted ? 'none' : 'accepted');
  await refresh();
}

export async function confirmOverride() {
  const id = app.confirmOverride;
  app.confirmOverride = null;
  if (!id) return;
  await store.setVerdict(id, 'accepted');
  await refresh();
  app.flash = `${id} · accepted over the agent's failure`;
}

export async function setVerdict(id: string, verdict: HumanVerdict) {
  await store.setVerdict(id, verdict);
  await refresh();
}

/** Work packages have a lifecycle, not a Status. */
export async function setWpState(id: string, state: WPState) {
  await store.setWpState(id, state);
  await refresh();
  app.flash = `${id} → ${state}`;
}

export async function submitComment(id: string) {
  const text = app.draftComment.trim();
  if (!text) return;
  app.draftComment = '';
  await store.addComment(id, text);
  await refresh();
}

// ── node writes ─────────────────────────────────────────────────────────────

export async function createNode(input: NewNode, selectIt = false) {
  const id = await store.createNode(input);
  await refresh();
  if (input.type === 'WORK_PACKAGE') app.expandedWp[id] = true;
  if (selectIt) app.selectedId = input.type === 'STEP' ? input.parentId : id;
  app.flash = `created ${id}`;
  return id;
}

export async function updateNode(id: string, patch: NodePatch) {
  await store.updateNode(id, patch);
  await refresh();
}

export async function deleteNode(id: string) {
  const wasSelected = app.selectedId === id;
  await store.deleteNode(id);
  await refresh();
  app.dialog = null;
  app.nodeMenuFor = null;
  if (wasSelected) app.selectedId = app.nodes.find((n) => n.type === 'TASK')?.id ?? null;
  app.flash = `deleted ${id} · undoable for 30s`;
}

export async function moveNode(id: string, newParentId: string, newType: NodeType) {
  await store.moveNode(id, newParentId, newType);
  await refresh();
  app.dialog = null;
  app.nodeMenuFor = null;
  app.selectedId = newType === 'STEP' ? newParentId : id;
  app.flash = `${id} → ${newType.toLowerCase().replace('_', ' ')}`;
}

export async function addDependency(blockerId: string, blockedId: string) {
  await store.addDependency(blockerId, blockedId);
  await refresh();
}

export async function removeDependency(blockerId: string, blockedId: string) {
  await store.removeDependency(blockerId, blockedId);
  await refresh();
}

// ── project writes ──────────────────────────────────────────────────────────

export async function createProject(name: string, description: string, seed: boolean) {
  const id = await store.createProject(name, description, seed);
  app.projects = await store.projects();
  app.dialog = null;
  await load(id);
}

export async function updateProject(id: string, patch: { name?: string; description?: string }) {
  await store.updateProject(id, patch);
  app.projects = await store.projects();
  app.dialog = null;
}

export async function archiveProject(id: string, archived: boolean) {
  await store.archiveProject(id, archived);
  app.projects = await store.projects();
  app.projectMenuOpen = false;
}

// ── ui ──────────────────────────────────────────────────────────────────────

export function toggleTheme() {
  app.theme = app.theme === 'dark' ? 'light' : 'dark';
  document.documentElement.dataset.theme = app.theme;
  remember(THEME_KEY, app.theme);
}

export function setPanelMode(mode: PanelMode) {
  app.panelMode = mode;
  remember(PANEL_KEY, mode);
}

export function togglePanelMode() {
  setPanelMode(app.panelMode === 'peek' ? 'expanded' : 'peek');
}

export function toggleWp(id: string) {
  app.expandedWp[id] = !app.expandedWp[id];
}

export function toggleTask(id: string) {
  app.expandedTask[id] = !app.expandedTask[id];
}

export function toggleStep(id: string) {
  app.expandedStep[id] = !app.expandedStep[id];
}

export function select(id: string | null, focus: FocusTarget = null) {
  app.selectedId = id;
  app.focusTarget = focus;
  if (id && isMobile()) app.sheet = focus ? 'full' : 'peek';
  if (!id) app.sheet = 'closed';
}

/**
 * Rename a step: select its owning task, then point the focus at the step.
 * Falls back to selecting the node itself if it has no parent.
 */
export function renameNode(node: FlowNode) {
  if (node.type === 'STEP' && node.parentId) {
    select(node.parentId, { section: 'steps', nodeId: node.id, field: 'title' });
    app.expandedStep[node.id] = true;
  } else {
    select(node.id, { section: 'title' });
  }
}

/** Read-and-clear, so the focus fires once per open rather than every render. */
export function takeFocusTarget(): FocusTarget {
  const t = app.focusTarget;
  app.focusTarget = null;
  return t;
}

export function closeDetail() {
  app.selectedId = null;
  app.focusTarget = null;
  app.sheet = 'closed';
}

export function closeOverlays() {
  app.dialog = null;
  app.filterOpen = false;
  app.projectMenuOpen = false;
  app.nodeMenuFor = null;
  app.confirmOverride = null;
}

// ── filters ─────────────────────────────────────────────────────────────────

export function toggleFilter(s: Status) {
  app.statusFilter = app.statusFilter.includes(s)
    ? app.statusFilter.filter((x) => x !== s)
    : [...app.statusFilter, s];
}

export function toggleWpFilter(id: string) {
  app.wpFilter = app.wpFilter.includes(id)
    ? app.wpFilter.filter((x) => x !== id)
    : [...app.wpFilter, id];
}

export function toggleVerFilter(key: string) {
  app.verFilter = app.verFilter.includes(key)
    ? app.verFilter.filter((x) => x !== key)
    : [...app.verFilter, key];
}

export function clearFilters() {
  app.statusFilter = [];
  app.wpFilter = [];
  app.verFilter = [];
  app.showArchived = false;
}

export function activeFilterCount(): number {
  return (
    app.statusFilter.length + app.wpFilter.length + app.verFilter.length + (app.showArchived ? 1 : 0)
  );
}

function verBucket(n: FlowNode): string {
  const v = n.verification;
  if (!v) return 'none';
  if (v.human === 'accepted' || v.agent === 'pass') return 'verified';
  if (v.agent === 'fail') return 'failed';
  if (v.agent === 'stale') return 'stale';
  return 'none';
}

/** One predicate for every view, so the three cannot disagree. */
export function passesAll(n: FlowNode): boolean {
  if (app.statusFilter.length && !app.statusFilter.includes(n.status)) return false;
  if (app.wpFilter.length) {
    const wpId = n.type === 'WORK_PACKAGE' ? n.id : n.type === 'TASK' ? n.parentId : null;
    if (!wpId || !app.wpFilter.includes(wpId)) return false;
  }
  if (app.verFilter.length && !app.verFilter.includes(verBucket(n))) return false;
  return true;
}

// ── graph zoom ──────────────────────────────────────────────────────────────

export const ZOOM_MIN = 0.35;
export const ZOOM_MAX = 2;

export function setZoom(z: number) {
  app.zoom = Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, Math.round(z * 100) / 100));
}

export function zoomBy(factor: number) {
  setZoom(app.zoom * factor);
}
