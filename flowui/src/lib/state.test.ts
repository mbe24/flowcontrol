import { Code, ConnectError } from '@connectrpc/connect';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { app, createNode, createProject, load, moveNode, passesAll, retryNow, setStore } from './state.svelte';
import type { FlowStore } from './store';

function mockStore() {
  return {
    projects: vi.fn(async () => []),
    nodes: vi.fn(async () => []),
    dependencies: vi.fn(async () => []),
    activity: vi.fn(async () => []),
    setStatus: vi.fn(async () => {}),
    setVerdict: vi.fn(async () => {}),
    addComment: vi.fn(async () => {}),
    createNode: vi.fn(async () => 'node-x'),
    updateNode: vi.fn(async () => {}),
    deleteNode: vi.fn(async () => {}),
    moveNode: vi.fn(async () => {}),
    addDependency: vi.fn(async () => {}),
    removeDependency: vi.fn(async () => {}),
    createProject: vi.fn(async () => 'prj-new'),
    updateProject: vi.fn(async () => {}),
    archiveProject: vi.fn(async () => {})
  } as unknown as FlowStore;
}

describe('new UI actions dispatch to the store', () => {
  beforeEach(() => {
    app.projectId = 'prj-travel';
    app.statusFilter = [];
    app.wpFilter = [];
    app.verFilter = [];
    app.nodes = [];
  });

  it('createNode delegates to the store and selects the new node', async () => {
    const store = mockStore();
    setStore(store);
    const id = await createNode({ projectId: 'prj-travel', parentId: 'WP-AUTH', type: 'TASK', title: 'Hello' }, true);
    expect(id).toBe('node-x');
    expect(store.createNode).toHaveBeenCalledWith(
      expect.objectContaining({ projectId: 'prj-travel', parentId: 'WP-AUTH', type: 'TASK', title: 'Hello' })
    );
    expect(app.selectedId).toBe('node-x');
  });

  it('moveNode delegates promote/demote/reparent', async () => {
    const store = mockStore();
    setStore(store);
    await moveNode('T-1042.1', 'WP-AUTH', 'TASK');
    expect(store.moveNode).toHaveBeenCalledWith('T-1042.1', 'WP-AUTH', 'TASK');
  });

  it('createProject delegates and reloads the new project', async () => {
    const store = mockStore();
    setStore(store);
    await createProject('New', 'desc', false);
    expect(store.createProject).toHaveBeenCalledWith('New', 'desc', false);
  });

  it('passesAll respects the status filter', () => {
    app.statusFilter = ['DONE'];
    expect(passesAll({ status: 'DONE' } as never)).toBe(true);
    expect(passesAll({ status: 'READY' } as never)).toBe(false);
  });
});

describe('degrade-and-reconnect on a transport drop', () => {
  beforeEach(() => {
    vi.useFakeTimers(); // freeze the reconnect poll; we drive retryNow() directly
    app.connection = 'connected';
    app.error = '';
    app.nodes = [];
  });
  afterEach(() => vi.useRealTimers());

  const down = () => async () => {
    throw new ConnectError('daemon gone', Code.Unavailable);
  };

  it('keeps the last snapshot and flips to disconnected, then recovers', async () => {
    const healthy = mockStore();
    (healthy.nodes as unknown) = vi.fn(async () => [{ id: 'T-1', type: 'TASK', status: 'READY' }]);
    setStore(healthy);
    await load('prj-travel');
    expect(app.connection).toBe('connected');
    expect(app.nodes).toHaveLength(1);

    // Daemon drops: every read throws Unavailable.
    const dead = mockStore();
    (dead.nodes as unknown) = vi.fn(down());
    (dead.dependencies as unknown) = vi.fn(down());
    (dead.activity as unknown) = vi.fn(down());
    setStore(dead);
    await load('prj-travel');
    expect(app.connection).toBe('disconnected');
    expect(app.nodes).toHaveLength(1); // last snapshot retained, NOT cleared
    expect(app.error).toBe(''); // no dead-end error

    // Daemon returns: an explicit retry reconnects and re-syncs.
    setStore(healthy);
    const ok = await retryNow();
    expect(ok).toBe(true);
    expect(app.connection).toBe('connected');
    expect(app.lastSyncedAt).toBeGreaterThan(0);
  });

  it('a real domain error does not trigger the disconnected banner', async () => {
    const store = mockStore();
    (store.nodes as unknown) = vi.fn(async () => {
      throw new ConnectError('bad argument', Code.InvalidArgument);
    });
    setStore(store);
    await load('prj-travel');
    expect(app.connection).toBe('connected'); // not a transport drop
    expect(app.error).toContain('bad argument');
  });
});
