import { describe, expect, it, vi } from 'vitest';
import {
  mapActivity,
  mapDependency,
  mapNode,
  mapProject,
  RemoteStore,
  relative,
  toDeclaredStatus,
  toVerdict
} from './remote';
import {
  AgentResult as PbAgentResult,
  DeclaredStatus as PbDeclared,
  EffectiveStatus as PbEffective,
  EventKind as PbEventKind,
  HumanVerdict as PbVerdict,
  NodeKind as PbKind,
  WorkPackageState as PbWpState
} from '../generated/flow/v1/flow_pb';
import type { Event as PbEvent, Node as PbNode, Project as PbProject } from '../generated/flow/v1/flow_pb';

const pbNode = (over: Partial<PbNode> = {}): PbNode =>
  ({
    id: 'T-1042',
    projectId: 'prj-travel',
    parentId: 'WP-AUTH',
    kind: PbKind.TASK,
    title: 'OAuth2 device-code flow',
    description: 'Line one\nLine two',
    condition: 'pnpm test:auth',
    reference: '',
    declaredStatus: 1,
    status: PbEffective.READY,
    wpState: 0,
    position: 0,
    verification: undefined,
    createdAt: 100n,
    updatedAt: 100n,
    ...over
  }) as unknown as PbNode;

describe('relative', () => {
  it('formats recent and older timestamps', () => {
    const now = Date.now() / 1000;
    expect(relative(now - 30)).toBe('just now');
    expect(relative(now - 120)).toBe('2m ago');
    expect(relative(now - 3600 * 3)).toBe('3h ago');
    expect(relative(now - 86400 * 5)).toBe('5d ago');
    expect(relative(0)).toBe('');
  });
});

describe('mapProject', () => {
  it('converts proto project to UI project', () => {
    const p = mapProject({ id: 'p1', name: 'N', description: 'D', createdAt: 123n, archivedAt: 0n } as unknown as PbProject);
    expect(p).toEqual({ id: 'p1', name: 'N', description: 'D', createdAt: 123 });
  });
});

describe('mapNode', () => {
  it('maps kind, effective status, and splits the markdown description into paragraphs', () => {
    const n = mapNode(pbNode());
    expect(n.type).toBe('TASK');
    expect(n.parentId).toBe('WP-AUTH');
    expect(n.status).toBe('READY');
    expect(n.description).toEqual(['Line one', 'Line two']);
    expect(n.condition).toBe('pnpm test:auth');
  });

  it('carries a work-package state', () => {
    const n = mapNode(pbNode({ kind: PbKind.WORK_PACKAGE, parentId: '', wpState: PbWpState.ACTIVE }));
    expect(n.type).toBe('WORK_PACKAGE');
    expect(n.state).toBe('ACTIVE');
    expect(n.parentId).toBeNull();
  });

  it('maps an agent result with a human verdict', () => {
    const n = mapNode(
      pbNode({
        verification: {
          agentResult: PbAgentResult.FAIL,
          agentName: 'claude-code',
          agentAt: 100n,
          agentDetail: 'boom',
          humanVerdict: PbVerdict.ACCEPTED,
          humanAt: 200n,
          stale: false
        } as unknown as NonNullable<PbNode['verification']>
      })
    );
    expect(n.verification?.agent).toBe('fail');
    expect(n.verification?.human).toBe('accepted');
  });

  it('flags a stale report', () => {
    const n = mapNode(
      pbNode({
        verification: {
          agentResult: PbAgentResult.PASS,
          agentName: '',
          agentAt: 100n,
          agentDetail: '',
          humanVerdict: PbVerdict.UNSPECIFIED,
          humanAt: 0n,
          stale: true
        } as unknown as NonNullable<PbNode['verification']>
      })
    );
    expect(n.verification?.agent).toBe('stale');
  });
});

describe('mapActivity', () => {
  const ev = (kind: PbEventKind): PbEvent =>
    ({ seq: 7n, projectId: 'prj-travel', nodeId: 'T-1042', kind, author: 'you', summary: 'hi', payloadJson: '{}', createdAt: 100n }) as unknown as PbEvent;

  it('maps status updates to status activity', () => {
    expect(mapActivity(ev(PbEventKind.STATUS_SET)).kind).toBe('status');
  });
  it('maps reports and verdicts to verify activity', () => {
    expect(mapActivity(ev(PbEventKind.AGENT_REPORTED)).kind).toBe('verify');
    expect(mapActivity(ev(PbEventKind.VERDICT_SET)).kind).toBe('verify');
  });
  it('maps comments', () => {
    expect(mapActivity(ev(PbEventKind.COMMENT)).kind).toBe('comment');
  });
  it('maps structural edits to edit activity', () => {
    for (const k of [PbEventKind.NODE_CREATED, PbEventKind.DEP_ADDED]) {
      expect(mapActivity(ev(k)).kind).toBe('edit');
    }
  });
});

describe('toDeclaredStatus / toVerdict', () => {
  it('only ever declares OPEN, DEFERRED or DONE', () => {
    expect(toDeclaredStatus('DONE')).toBe(PbDeclared.DONE);
    expect(toDeclaredStatus('DEFERRED')).toBe(PbDeclared.DEFERRED);
    expect(toDeclaredStatus('READY')).toBe(PbDeclared.OPEN);
    expect(toDeclaredStatus('BLOCKED')).toBe(PbDeclared.OPEN);
  });
  it('maps verdicts', () => {
    expect(toVerdict('accepted')).toBe(PbVerdict.ACCEPTED);
    expect(toVerdict('rejected')).toBe(PbVerdict.REJECTED);
    expect(toVerdict('none')).toBe(PbVerdict.UNSPECIFIED);
  });
});

describe('RemoteStore', () => {
  const snap = {
    project: null,
    nodes: [pbNode()],
    dependencies: [{ blockerId: 'T-1042', blockedId: 'T-1043' }],
    progress: [],
    recentEvents: [{ seq: 1n, projectId: 'prj-travel', nodeId: 'T-1042', kind: PbEventKind.STATUS_SET, author: 'you', summary: 'READY -> DONE', payloadJson: '{}', createdAt: 100n }],
    seq: 1n
  };

  function fakeClient(overrides: Record<string, unknown> = {}) {
    const calls = { setStatus: [], setVerdict: [], addComment: [] } as Record<string, unknown[]>;
    const client = {
      listProjects: vi.fn(async () => ({ projects: [{ id: 'p1', name: 'N', description: 'D', createdAt: 1n, archivedAt: 0n }] })),
      getSnapshot: vi.fn(async () => snap),
      setStatus: vi.fn(async (req: unknown) => {
        calls.setStatus.push(req);
      }),
      setVerdict: vi.fn(async (req: unknown) => {
        calls.setVerdict.push(req);
      }),
      addComment: vi.fn(async (req: unknown) => {
        calls.addComment.push(req);
      })
    };
    return { client, calls, ...overrides };
  }

  function storeFor(c: any) {
    return new RemoteStore(c);
  }

  it('projects maps the response', async () => {
    const { client } = fakeClient();
    const projects = await storeFor(client).projects();
    expect(projects).toHaveLength(1);
    expect(projects[0].name).toBe('N');
  });

  it('nodes, dependencies and activity all come from one snapshot', async () => {
    const { client } = fakeClient();
    const store = storeFor(client);
    await store.nodes('prj-travel');
    await store.dependencies('prj-travel');
    await store.activity('prj-travel');
    expect(client.getSnapshot).toHaveBeenCalledTimes(3);
    expect((await store.dependencies('prj-travel'))[0].blockedId).toBe('T-1043');
  });

  it('setStatus declares OPEN when the UI asks for READY', async () => {
    const { client } = fakeClient();
    await storeFor(client).setStatus('T-1042', 'READY');
    const req = (client.setStatus as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(req.nodeId).toBe('T-1042');
    expect(req.declaredStatus).toBe(PbDeclared.OPEN);
  });

  it('setVerdict maps to the accepted verdict', async () => {
    const { client } = fakeClient();
    await storeFor(client).setVerdict('T-1042', 'accepted');
    const req = (client.setVerdict as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(req.verdict).toBe(PbVerdict.ACCEPTED);
  });

  it('addComment passes the text through', async () => {
    const { client } = fakeClient();
    await storeFor(client).addComment('T-1042', 'hello');
    const req = (client.addComment as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(req.text).toBe('hello');
  });
});
