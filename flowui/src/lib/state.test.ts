import { beforeEach, describe, expect, it, vi } from 'vitest';
import { app, createNode, createProject, moveNode, passesAll, setStore } from './state.svelte';
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
