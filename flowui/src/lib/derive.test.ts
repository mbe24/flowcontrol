import { describe, expect, it } from 'vitest';
import { buildIndex, countStatuses, hueOf, layoutGraph, ownerTask, projectCounts, stepRatio, workPackages, wpCounts } from './derive';
import type { Dependency, FlowNode } from './types';

const wp: FlowNode = {
  id: 'WP-1',
  projectId: 'p',
  parentId: null,
  type: 'WORK_PACKAGE',
  title: 'WP',
  description: [],
  status: 'READY',
  state: 'ACTIVE'
};

const t = (id: string, parentId: string, status: FlowNode['status']): FlowNode => ({
  id,
  projectId: 'p',
  parentId,
  type: 'TASK',
  title: id,
  description: [],
  status
});

const s = (id: string, parentId: string, status: FlowNode['status']): FlowNode => ({
  id,
  projectId: 'p',
  parentId,
  type: 'STEP',
  title: id,
  description: [],
  status
});

describe('countStatuses', () => {
  it('counts each status and computes percentage', () => {
    const c = countStatuses([t('a', 'X', 'DONE'), t('b', 'X', 'READY'), t('c', 'X', 'BLOCKED'), t('d', 'X', 'DEFERRED')]);
    expect(c).toEqual({ done: 1, ready: 1, blocked: 1, deferred: 1, total: 4, pct: 25 });
  });

  it('is 0 percent for an empty set', () => {
    const c = countStatuses([]);
    expect(c.total).toBe(0);
    expect(c.pct).toBe(0);
  });
});

describe('workPackages', () => {
  it('sorts ACTIVE packages first', () => {
    const planned: FlowNode = { ...wp, id: 'W2', state: 'PLANNED' };
    const out = workPackages([planned, wp]);
    expect(out[0].id).toBe('WP-1');
    expect(out[1].id).toBe('W2');
  });
});

describe('wpCounts / stepRatio', () => {
  it('counts leaves below a package (steps win over their task)', () => {
    const nodes = [wp, t('T1', 'WP-1', 'DONE'), s('T1.1', 'T1', 'DONE'), s('T1.2', 'T1', 'BLOCKED')];
    const c = wpCounts(nodes, 'WP-1');
    expect(c.total).toBe(2);
    expect(c.done).toBe(1);
    expect(c.blocked).toBe(1);
    expect(stepRatio(nodes, 'T1').label).toBe('1/2');
  });
});

describe('projectCounts', () => {
  it('excludes work packages from project totals', () => {
    const nodes = [wp, t('T1', 'WP-1', 'DONE')];
    const c = projectCounts(nodes);
    expect(c.total).toBe(1);
    expect(c.done).toBe(1);
  });
});

describe('buildIndex / ownerTask', () => {
  it('builds blocker and blocked maps from dependencies', () => {
    const deps: Dependency[] = [{ blockerId: 'A', blockedId: 'B' }];
    const idx = buildIndex([t('A', 'W', 'DONE'), t('B', 'W', 'READY')], deps);
    expect(idx.blockers.get('B')).toEqual(['A']);
    expect(idx.blocks.get('A')).toEqual(['B']);
  });

  it('resolves a step to its owning task', () => {
    const step = s('S1', 'T1', 'READY');
    const task = t('T1', 'WP-1', 'READY');
    const idx = buildIndex([step, task], []);
    expect(ownerTask(idx, step).id).toBe('T1');
  });
});

describe('layoutGraph', () => {
  it('turns an ACTIVE package into an expanded cluster and the rest into boxes', () => {
    const collapsed: FlowNode = { ...wp, id: 'WP-2', state: 'PLANNED' };
    const nodes = [wp, collapsed, t('T1', 'WP-1', 'READY')];
    const deps: Dependency[] = [];
    const g = layoutGraph(nodes, deps, new Set(['WP-1']));
    expect(g.clusters.length).toBe(1);
    expect(g.clusters[0].wp.id).toBe('WP-1');
    expect(g.boxes.length).toBe(1);
    expect(g.boxes[0].wp.id).toBe('WP-2');
    expect(g.width).toBeGreaterThan(0);
    expect(g.height).toBeGreaterThan(0);
  });
});

describe('hueOf', () => {
  it('cycles through hues deterministically', () => {
    expect(hueOf([wp, { ...wp, id: 'W2' }], 'WP-1')).toBe('var(--hue-auth)');
  });
});
