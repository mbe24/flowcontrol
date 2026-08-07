import { beforeEach, describe, expect, it } from 'vitest';
import { MemoryStore } from './memory';

let store: MemoryStore;

beforeEach(() => {
  store = new MemoryStore();
});

describe('MemoryStore (new design)', () => {
  it('lists projects', async () => {
    const ps = await store.projects();
    expect(ps.length).toBeGreaterThan(0);
  });

  it('returns nodes for a project', async () => {
    const ns = await store.nodes('prj-travel');
    expect(ns.length).toBeGreaterThan(0);
  });

  it('createNode adds a node and returns its id', async () => {
    const id = await store.createNode({ projectId: 'prj-travel', parentId: 'WP-AUTH', type: 'TASK', title: 'New task' });
    const n = (await store.nodes('prj-travel')).find((x) => x.id === id);
    expect(n?.title).toBe('New task');
    expect(n?.type).toBe('TASK');
  });

  it('updateNode edits provided fields', async () => {
    await store.updateNode('T-1042', { title: 'Renamed', condition: 'pnpm test' });
    const n = (await store.nodes('prj-travel')).find((x) => x.id === 'T-1042');
    expect(n?.title).toBe('Renamed');
    expect(n?.condition).toBe('pnpm test');
  });

  it('moveNode promotes a step to a task under the package', async () => {
    await store.moveNode('T-1042.1', 'WP-AUTH', 'TASK');
    const moved = (await store.nodes('prj-travel')).find((x) => x.id === 'T-1042.1');
    expect(moved?.type).toBe('TASK');
    expect(moved?.parentId).toBe('WP-AUTH');
  });

  it('deleteNode removes the node', async () => {
    await store.deleteNode('T-1042.1');
    expect((await store.nodes('prj-travel')).some((n) => n.id === 'T-1042.1')).toBe(false);
  });

  it('add/removeDependency mutate the edge set', async () => {
    const before = (await store.dependencies('prj-travel')).length;
    await store.addDependency('T-1042', 'T-1044');
    expect((await store.dependencies('prj-travel')).length).toBe(before + 1);
    await store.removeDependency('T-1042', 'T-1044');
    expect((await store.dependencies('prj-travel')).length).toBe(before);
  });

  it('project create/update/archive', async () => {
    const id = await store.createProject('New project', 'desc', false);
    expect((await store.projects()).some((p) => p.id === id)).toBe(true);
    await store.updateProject(id, { name: 'Renamed' });
    expect((await store.projects()).find((p) => p.id === id)?.name).toBe('Renamed');
    await store.archiveProject(id, true);
    expect((await store.projects()).find((p) => p.id === id)?.archived).toBe(true);
  });
});
