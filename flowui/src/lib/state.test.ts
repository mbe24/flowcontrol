import { beforeEach, describe, expect, it, vi } from 'vitest';
import { app, createTask, setStore, setStatus, undo } from './state.svelte';
import type { FlowStore } from './store';

const wp = { id: 'WP-A', projectId: 'prj-travel', parentId: null, type: 'WORK_PACKAGE', title: 'Auth', description: [], status: 'READY', state: 'ACTIVE' };

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
    deleteNode: vi.fn(async () => {}),
    addDependency: vi.fn(async () => {}),
    removeDependency: vi.fn(async () => {}),
    undo: vi.fn(async () => {})
  } as unknown as FlowStore;
}

// The UI actions in state.svelte.ts are the only place components touch the
// store. Mocking FlowStore here proves each action dispatches the right call,
// so a real store (RemoteStore) forwards it to the flowd RPC.
describe('UI actions dispatch to the store', () => {
  beforeEach(() => {
    app.projectId = 'prj-travel';
    app.taskTitle = '';
    app.taskDialog = false;
    app.nodes = [wp] as never;
  });

  it('createTask creates a TASK under the first work package', async () => {
    const store = mockStore();
    setStore(store);
    app.taskTitle = 'Write docs';
    await createTask();
    expect(store.createNode).toHaveBeenCalledTimes(1);
    expect(store.createNode).toHaveBeenCalledWith(
      expect.objectContaining({ projectId: 'prj-travel', parentId: 'WP-A', kind: 'TASK', title: 'Write docs' })
    );
  });

  it('undo calls the server-side store.undo for the current project', async () => {
    const store = mockStore();
    setStore(store);
    await undo();
    expect(store.undo).toHaveBeenCalledWith('prj-travel');
  });

  it('setStatus delegates to the store', async () => {
    const store = mockStore();
    setStore(store);
    app.nodes = [{ id: 'X', projectId: 'prj-travel', parentId: 'WP-A', type: 'TASK', title: 'X', description: [], status: 'READY' }] as never;
    await setStatus('X', 'DONE');
    expect(store.setStatus).toHaveBeenCalledWith('X', 'DONE');
  });
});
