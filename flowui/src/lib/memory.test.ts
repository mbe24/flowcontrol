import { beforeEach, describe, expect, it } from 'vitest';
import { MemoryStore } from './memory';

let store: MemoryStore;

beforeEach(() => {
  store = new MemoryStore();
});

describe('MemoryStore projects', () => {
  it('lists all fixtures', async () => {
    const projects = await store.projects();
    expect(projects.some((p) => p.id === 'prj-travel')).toBe(true);
  });
});

describe('MemoryStore nodes', () => {
  it('filters nodes by project', async () => {
    const nodes = await store.nodes('prj-travel');
    expect(nodes.length).toBeGreaterThan(0);
    expect(nodes.every((n) => n.projectId === 'prj-travel')).toBe(true);
  });
});

describe('MemoryStore writes', () => {
  it('setStatus flips the node and records status activity', async () => {
    await store.setStatus('T-1042', 'DONE');
    const nodes = await store.nodes('prj-travel');
    expect(nodes.find((n) => n.id === 'T-1042')?.status).toBe('DONE');
    const activity = await store.activity('prj-travel');
    expect(activity[0].kind).toBe('status');
  });

  it('setVerdict records an acceptance', async () => {
    await store.setVerdict('T-1042', 'accepted');
    const [n] = (await store.nodes('prj-travel')).filter((x) => x.id === 'T-1042');
    expect(n.verification?.human).toBe('accepted');
  });

  it('addComment prepends a comment entry', async () => {
    const before = await store.activity('prj-travel');
    await store.addComment('T-1042', 'new note');
    const after = await store.activity('prj-travel');
    expect(after[0].kind).toBe('comment');
    expect(after[0].text).toBe('new note');
    expect(after.length).toBe(before.length + 1);
  });

  it('setStatus on a missing node rejects', async () => {
    await expect(store.setStatus('nope', 'DONE')).rejects.toThrow(/not found/);
  });

  it('updateNode changes the editable fields', async () => {
    await store.updateNode('T-1042', { title: 'Renamed', condition: 'pnpm test' });
    const [n] = (await store.nodes('prj-travel')).filter((x) => x.id === 'T-1042');
    expect(n.title).toBe('Renamed');
    expect(n.condition).toBe('pnpm test');
  });
});
