import { describe, expect, it, vi } from 'vitest';
import { mapActivity, mapNode, mapProject, RemoteStore } from './remote';
import {
  AgentResult as PbAgentResult,
  DeclaredStatus as PbDeclared,
  EffectiveStatus as PbEffective,
  EventKind as PbEventKind,
  HumanVerdict as PbVerdict,
  NodeKind as PbKind,
  WorkPackageState as PbWpState
} from '@flow/api/flow/v1/flow_pb';
import type { Event as PbEvent, Node as PbNode, Project as PbProject } from '@flow/api/flow/v1/flow_pb';

const pbNode = (over: Partial<PbNode> = {}): PbNode =>
  ({
    id: 'T-1042',
    projectId: 'prj-travel',
    parentId: 'WP-AUTH',
    kind: PbKind.TASK,
    title: 'Auth flow',
    description: 'One\n\nTwo',
    condition: 'pnpm test:auth',
    reference: '',
    declaredStatus: 1,
    status: PbEffective.READY,
    wpState: 0,
    position: 0,
    createdAt: 100n,
    updatedAt: 100n,
    ...over
  }) as unknown as PbNode;

describe('mappers', () => {
  it('mapProject and mapNode convert proto to UI shape', () => {
    expect(mapProject({ id: 'p', name: 'N', description: 'D', createdAt: 1n, archivedAt: 0n } as PbProject).name).toBe('N');
    expect(mapProject({ id: 'p', name: 'N', description: 'D', createdAt: 1n, archivedAt: 0n } as PbProject).archived).toBe(false);
    expect(mapProject({ id: 'p', name: 'N', description: 'D', createdAt: 1n, archivedAt: 9n } as PbProject).archived).toBe(true);
    const n = mapNode(pbNode({ note: 'step body' } as Partial<PbNode>));
    expect(n.type).toBe('TASK');
    expect(n.parentId).toBe('WP-AUTH');
    expect(n.description).toEqual(['One', 'Two']);
    expect(n.status).toBe('READY');
    expect(n.condition).toBe('pnpm test:auth');
    expect(n.note).toBe('step body');
  });

  it('mapActivity maps event kinds', () => {
    expect(mapActivity({ seq: 1n, nodeId: 'x', kind: PbEventKind.STATUS_SET, author: 'a', summary: 's', payloadJson: '{}', createdAt: 1n } as PbEvent).kind).toBe('status');
  });
});

function fakeClient() {
  const calls: Record<string, unknown[]> = {};
  const fn = (name: string) =>
    vi.fn(async (req: unknown) => {
      (calls[name] ??= []).push(req);
      if (name === 'createNode') return { mutation: { changedNodes: [{ id: 'node-new' }] } };
      if (name === 'createProject') return { project: { id: 'prj-new' } };
    });
  const client = {
    listProjects: fn('listProjects'),
    getSnapshot: fn('getSnapshot'),
    setStatus: fn('setStatus'),
    setVerdict: fn('setVerdict'),
    addComment: fn('addComment'),
    createNode: fn('createNode'),
    updateNode: fn('updateNode'),
    deleteNode: fn('deleteNode'),
    addDependency: fn('addDependency'),
    removeDependency: fn('removeDependency'),
    moveNode: fn('moveNode'),
    createProject: fn('createProject'),
    updateProject: fn('updateProject'),
    archiveProject: fn('archiveProject')
  };
  return { client, calls };
}

function storeWith(client: ReturnType<typeof fakeClient>['client']) {
  return new RemoteStore(client as never);
}

describe('RemoteStore', () => {
  const snap = {
    project: null,
    nodes: [pbNode()],
    dependencies: [{ blockerId: 'T-1042', blockedId: 'T-1043' }],
    progress: [],
    recentEvents: [
      { seq: 1n, projectId: 'prj-travel', nodeId: 'T-1042', kind: PbEventKind.STATUS_SET, author: 'you', summary: 'READY -> DONE', payloadJson: '{}', createdAt: 100n }
    ],
    seq: 1n
  };

  it('createNode maps type/title and returns the created id', async () => {
    const { client } = fakeClient();
    const store = storeWith(client);
    const id = await store.createNode({ projectId: 'prj-travel', parentId: 'WP-AUTH', type: 'TASK', title: 'New' });
    expect(id).toBe('node-new');
    const req = (client.createNode as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(req.kind).toBe(PbKind.TASK);
    expect(req.parentId).toBe('WP-AUTH');
    expect(req.title).toBe('New');
  });

  it('setStatus declares OPEN for READY', async () => {
    const { client } = fakeClient();
    const store = storeWith(client);
    await store.setStatus('T-1042', 'READY');
    const req = (client.setStatus as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(req.declaredStatus).toBe(PbDeclared.OPEN);
  });

  it('setWpState maps to an update with a wp_state mask', async () => {
    const { client } = fakeClient();
    const store = storeWith(client);
    await store.setWpState('WP-AUTH', 'ACTIVE');
    const req = (client.updateNode as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(req.updateMask).toEqual(['wp_state']);
    expect(req.wpState).toBe(PbWpState.ACTIVE);
  });

  it('updateNode sends a mask for only provided fields (incl. note)', async () => {
    const { client } = fakeClient();
    const store = storeWith(client);
    await store.updateNode('T-1042', { title: 'Renamed', note: 'body' });
    const req = (client.updateNode as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(req.updateMask).toEqual(['title', 'note']);
    expect(req.title).toBe('Renamed');
    expect(req.note).toBe('body');
  });

  it('backed writes hit their RPCs', async () => {
    const { client } = fakeClient();
    const store = storeWith(client);
    await store.deleteNode('T-1042');
    await store.addDependency('A', 'B');
    await store.removeDependency('A', 'B');
    expect(client.deleteNode).toHaveBeenCalledTimes(1);
    expect(client.addDependency).toHaveBeenCalledTimes(1);
    expect(client.removeDependency).toHaveBeenCalledTimes(1);
  });

  it('moveNode maps node_id/parent/kind to a MoveNode call', async () => {
    const { client } = fakeClient();
    const store = storeWith(client);
    await store.moveNode('T-1042', 'WP-X', 'STEP');
    const req = (client.moveNode as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(req.nodeId).toBe('T-1042');
    expect(req.parentId).toBe('WP-X');
    expect(req.kind).toBe(PbKind.STEP);
  });

  it('createProject returns the new id and seeds a work package when asked', async () => {
    const { client } = fakeClient();
    const store = storeWith(client);
    const id = await store.createProject('P', 'd', true);
    expect(id).toBe('prj-new');
    // seed = a follow-up createNode(WORK_PACKAGE) under the new project
    const seed = (client.createNode as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(seed.projectId).toBe('prj-new');
    expect(seed.kind).toBe(PbKind.WORK_PACKAGE);
  });

  it('updateProject masks provided fields; archiveProject toggles', async () => {
    const { client } = fakeClient();
    const store = storeWith(client);
    await store.updateProject('p', { name: 'x' });
    expect((client.updateProject as ReturnType<typeof vi.fn>).mock.calls[0][0].updateMask).toEqual(['name']);
    await store.archiveProject('p', true);
    expect((client.archiveProject as ReturnType<typeof vi.fn>).mock.calls[0][0].archived).toBe(true);
  });
});