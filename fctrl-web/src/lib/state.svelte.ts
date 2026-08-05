import { MemoryStore } from './memory';
import type { FlowStore } from './store';
import type { Dependency, FlowNode, Project, Status } from './types';

/** Swap this for a client that talks to the Rust core. */
export const store: FlowStore = new MemoryStore();

export type ViewName = 'table' | 'lanes' | 'graph';

export const app = $state({
  loading: true,
  error: '' as string,

  projects: [] as Project[],
  projectId: 'prj-travel',
  nodes: [] as FlowNode[],
  deps: [] as Dependency[],

  view: 'table' as ViewName,
  theme: 'dark' as 'dark' | 'light',
  selectedId: 'T-1042' as string | null,

  paletteOpen: false,
  paletteQuery: '',
  editMode: false,
  showArchived: false,
  statusFilter: [] as Status[],

  expandedWp: {} as Record<string, boolean>,
  expandedTask: { 'T-1042': true } as Record<string, boolean>,

  verifying: false,
  flash: '',
});

export async function boot() {
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
    const [nodes, deps] = await Promise.all([
      store.nodes(projectId),
      store.dependencies(projectId),
    ]);
    app.nodes = nodes;
    app.deps = deps;
    for (const n of nodes) {
      if (n.type === 'WORK_PACKAGE' && n.state === 'ACTIVE')
        app.expandedWp[n.id] = true;
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
  const [nodes, deps] = await Promise.all([
    store.nodes(app.projectId),
    store.dependencies(app.projectId),
  ]);
  app.nodes = nodes;
  app.deps = deps;
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

export async function verify(id: string) {
  app.verifying = true;
  app.flash = '';
  try {
    await store.verify(id);
    await refresh();
  } finally {
    app.verifying = false;
  }
}

export function toggleTheme() {
  app.theme = app.theme === 'dark' ? 'light' : 'dark';
  document.documentElement.dataset.theme = app.theme;
}

export function toggleWp(id: string) {
  app.expandedWp[id] = !app.expandedWp[id];
}

export function toggleTask(id: string) {
  app.expandedTask[id] = !app.expandedTask[id];
}

export function select(id: string | null) {
  app.selectedId = id;
}

export function toggleFilter(s: Status) {
  app.statusFilter = app.statusFilter.includes(s)
    ? app.statusFilter.filter((x) => x !== s)
    : [...app.statusFilter, s];
}

export function passesFilter(status: Status): boolean {
  return app.statusFilter.length === 0 || app.statusFilter.includes(status);
}
