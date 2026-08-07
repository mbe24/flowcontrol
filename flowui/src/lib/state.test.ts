import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  app,
  confirmOverride,
  createTask,
  passesFilter,
  setStore,
  setStatus,
  setVerdict,
  submitComment,
  toggleFilter,
  toggleVerified,
  undo,
  updateNode
} from './state.svelte';
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
    updateNode: vi.fn(async () => {}),
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

  it('updateNode delegates the patch to the store', async () => {
    const store = mockStore();
    setStore(store);
    await updateNode('X', { title: 'Renamed' });
    expect(store.updateNode).toHaveBeenCalledWith('X', { title: 'Renamed' });
  });
});

describe('further UI actions', () => {
  beforeEach(() => {
    app.projectId = 'prj-travel';
    app.confirmOverride = null;
    app.draftComment = '';
    app.statusFilter = [];
    app.nodes = [wp] as never;
  });

  it('submitComment trims, clears the draft and delegates to store.addComment', async () => {
    const store = mockStore();
    setStore(store);
    app.draftComment = '  hello  ';
    await submitComment('T-1042');
    expect(app.draftComment).toBe('');
    expect(store.addComment).toHaveBeenCalledWith('T-1042', 'hello');
  });

  it('submitComment ignores a blank draft', async () => {
    const store = mockStore();
    setStore(store);
    app.draftComment = '   ';
    await submitComment('T-1042');
    expect(store.addComment).not.toHaveBeenCalled();
  });

  it('toggleVerified on a reported failure defers to the confirm dialog', async () => {
    const store = mockStore();
    setStore(store);
    app.nodes = [
      {
        id: 'X',
        projectId: 'prj-travel',
        parentId: 'WP-A',
        type: 'TASK',
        title: 'X',
        description: [],
        status: 'READY',
        verification: { agent: 'fail', agentName: 'claude', agentWhen: '', human: 'none', humanWhen: '' }
      }
    ] as never;
    await toggleVerified('X');
    expect(app.confirmOverride).toBe('X');
    expect(store.setVerdict).not.toHaveBeenCalled();
  });

  it('toggleVerified on an accepted node clears the override', async () => {
    const store = mockStore();
    setStore(store);
    app.nodes = [
      {
        id: 'X',
        projectId: 'prj-travel',
        parentId: 'WP-A',
        type: 'TASK',
        title: 'X',
        description: [],
        status: 'READY',
        verification: { agent: 'pass', agentName: '', agentWhen: '', human: 'accepted', humanWhen: '' }
      }
    ] as never;
    await toggleVerified('X');
    expect(store.setVerdict).toHaveBeenCalledWith('X', 'none');
  });

  it('confirmOverride records an acceptance over the agent failure', async () => {
    const store = mockStore();
    setStore(store);
    app.confirmOverride = 'X';
    await confirmOverride();
    expect(app.confirmOverride).toBeNull();
    expect(store.setVerdict).toHaveBeenCalledWith('X', 'accepted');
  });

  it('setVerdict delegates directly', async () => {
    const store = mockStore();
    setStore(store);
    await setVerdict('X', 'rejected');
    expect(store.setVerdict).toHaveBeenCalledWith('X', 'rejected');
  });

  it('toggleFilter toggles a status in the filter set', () => {
    expect(passesFilter('DONE')).toBe(true);
    toggleFilter('DONE');
    expect(passesFilter('DONE')).toBe(true);
    expect(passesFilter('READY')).toBe(false);
    toggleFilter('DONE');
    expect(passesFilter('DONE')).toBe(true);
  });
});
