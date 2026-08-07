import { MemoryStore } from './memory';
import { RemoteStore } from './remote';
import type { FlowStore } from './store';
import type { ActivityEntry, Dependency, FlowNode, HumanVerdict, Project, Status } from './types';

/**
 * The seam: the demo build keeps the in-memory fixtures, the real build (or a
 * plain `vite dev`) talks to the running core. Binary switch so tree-shaking
 * keeps the fixture data out of the production bundle.
 */
export const store: FlowStore = import.meta.env.VITE_DEMO ? new MemoryStore() : new RemoteStore();

export type ViewName = 'table' | 'lanes' | 'graph';
/** Desktop: how much room the detail surface takes. Persisted. */
export type PanelMode = 'peek' | 'expanded';
/** Mobile: where the sheet is resting. */
export type SheetMode = 'closed' | 'peek' | 'full';

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
  /** Desktop panel width while dragging the left edge. */
  dragging: false,

  paletteOpen: false,
  paletteQuery: '',
  editMode: false,
  showArchived: false,
  statusFilter: [] as Status[],

  /** Lanes view on mobile shows one lane at a time. */
  laneIndex: 0,

  expandedWp: {} as Record<string, boolean>,
  expandedTask: { 'T-1042': true } as Record<string, boolean>,
  expandedStep: {} as Record<string, boolean>,

  /** Set when a human override would contradict an agent failure. */
  confirmOverride: null as string | null,
  draftComment: '',
  flash: ''
});

export const isMobile = () => app.width < MOBILE_BP;

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
    app.error = String(e);
  }
}

export async function load(projectId: string) {
  app.loading = true;
  app.projectId = projectId;
  try {
    const [nodes, deps, activity] = await Promise.all([
      store.nodes(projectId),
      store.dependencies(projectId),
      store.activity(projectId)
    ]);
    app.nodes = nodes;
    app.deps = deps;
    app.activity = activity;
    for (const n of nodes) {
      if (n.type === 'WORK_PACKAGE' && n.state === 'ACTIVE') app.expandedWp[n.id] = true;
    }
    if (app.selectedId && !nodes.some((n) => n.id === app.selectedId)) {
      app.selectedId = nodes.find((n) => n.type === 'TASK')?.id ?? null;
    }
  } catch (e) {
    app.error = String(e);
  } finally {
    app.loading = false;
  }
}

async function refresh() {
  const [nodes, deps, activity] = await Promise.all([
    store.nodes(app.projectId),
    store.dependencies(app.projectId),
    store.activity(app.projectId)
  ]);
  app.nodes = nodes;
  app.deps = deps;
  app.activity = activity;
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

export async function submitComment(id: string) {
  const text = app.draftComment.trim();
  if (!text) return;
  app.draftComment = '';
  await store.addComment(id, text);
  await refresh();
}

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

export function select(id: string | null) {
  app.selectedId = id;
  if (id && isMobile()) app.sheet = 'peek';
  if (!id) app.sheet = 'closed';
}

export function closeDetail() {
  app.selectedId = null;
  app.sheet = 'closed';
}

export function toggleFilter(s: Status) {
  app.statusFilter = app.statusFilter.includes(s)
    ? app.statusFilter.filter((x) => x !== s)
    : [...app.statusFilter, s];
}

export function passesFilter(status: Status): boolean {
  return app.statusFilter.length === 0 || app.statusFilter.includes(status);
}
