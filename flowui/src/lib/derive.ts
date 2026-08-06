import type { Dependency, FlowNode, Status } from './types';

export interface Index {
  byId: Map<string, FlowNode>;
  blockers: Map<string, string[]>;
  blocks: Map<string, string[]>;
}

export function buildIndex(nodes: FlowNode[], deps: Dependency[]): Index {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const blockers = new Map<string, string[]>();
  const blocks = new Map<string, string[]>();
  for (const d of deps) {
    blockers.set(d.blockedId, [...(blockers.get(d.blockedId) ?? []), d.blockerId]);
    blocks.set(d.blockerId, [...(blocks.get(d.blockerId) ?? []), d.blockedId]);
  }
  return { byId, blockers, blocks };
}

export const workPackages = (nodes: FlowNode[]) =>
  nodes.filter((n) => n.type === 'WORK_PACKAGE').sort((a, b) => statePriority(a) - statePriority(b));

export const tasksOf = (nodes: FlowNode[], wpId: string) =>
  nodes.filter((n) => n.type === 'TASK' && n.parentId === wpId);

export const stepsOf = (nodes: FlowNode[], taskId: string) =>
  nodes.filter((n) => n.type === 'STEP' && n.parentId === taskId);

function statePriority(n: FlowNode): number {
  switch (n.state) {
    case 'ACTIVE':
      return 0;
    case 'PLANNED':
      return 1;
    case 'DONE':
      return 2;
    default:
      return 3;
  }
}

export interface Counts {
  done: number;
  ready: number;
  blocked: number;
  deferred: number;
  total: number;
  pct: number;
}

export function countStatuses(nodes: FlowNode[]): Counts {
  const c: Counts = { done: 0, ready: 0, blocked: 0, deferred: 0, total: 0, pct: 0 };
  for (const n of nodes) {
    c.total++;
    if (n.status === 'DONE') c.done++;
    else if (n.status === 'READY') c.ready++;
    else if (n.status === 'BLOCKED') c.blocked++;
    else c.deferred++;
  }
  c.pct = c.total ? Math.round((c.done / c.total) * 100) : 0;
  return c;
}

/** Counts every leaf below a work package: tasks plus their steps. */
export function wpCounts(nodes: FlowNode[], wpId: string): Counts {
  const tasks = tasksOf(nodes, wpId);
  const leaves: FlowNode[] = [];
  for (const t of tasks) {
    const steps = stepsOf(nodes, t.id);
    if (steps.length) leaves.push(...steps);
    else leaves.push(t);
  }
  return countStatuses(leaves);
}

export function projectCounts(nodes: FlowNode[]): Counts {
  return countStatuses(nodes.filter((n) => n.type !== 'WORK_PACKAGE'));
}

export function stepRatio(nodes: FlowNode[], taskId: string): { done: number; total: number; label: string } {
  const steps = stepsOf(nodes, taskId);
  const done = steps.filter((s) => s.status === 'DONE').length;
  return { done, total: steps.length, label: steps.length ? `${done}/${steps.length}` : '–' };
}

const HUES = ['var(--hue-auth)', 'var(--hue-booking)', 'var(--hue-pay)', 'var(--hue-obs)', 'var(--hue-ui)'];

export function hueOf(nodes: FlowNode[], wpId: string): string {
  const i = workPackages(nodes).findIndex((w) => w.id === wpId);
  return HUES[(i < 0 ? 0 : i) % HUES.length];
}

/** The task (or work package) a status change should apply to. */
export function ownerTask(index: Index, node: FlowNode): FlowNode {
  if (node.type === 'STEP' && node.parentId) {
    return index.byId.get(node.parentId) ?? node;
  }
  return node;
}

// ── graph layout ───────────────────────────────────────────────────────────

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface GraphNode extends Rect {
  node: FlowNode;
  hue: string;
  steps: string;
}

export interface GraphCluster extends Rect {
  wp: FlowNode;
  hue: string;
  counts: Counts;
}

export interface GraphBox extends Rect {
  wp: FlowNode;
  hue: string;
  counts: Counts;
}

export interface GraphEdge {
  id: string;
  d: string;
  tx: number;
  ty: number;
  kind: 'blocked' | 'satisfied' | 'rollup';
  from: string;
  to: string;
  label?: string;
  mx: number;
  my: number;
}

export interface Graph {
  width: number;
  height: number;
  clusters: GraphCluster[];
  nodes: GraphNode[];
  boxes: GraphBox[];
  edges: GraphEdge[];
}

const NODE_W = 154;
const NODE_H = 46;
const COL_GAP = 30;
const ROW_GAP = 26;
const PAD = 18;
const HEADER = 34;
const CLUSTER_GAP = 44;
const BOX_W = 180;
const BOX_H = 74;
const BOX_GAP = 26;
const LEFT_COL = 20;
const CLUSTER_X = LEFT_COL + BOX_W + 76;

/**
 * Lays out the active work packages as expanded clusters and everything else as
 * collapsed boxes down the left. Cross-level and cross-package dependencies
 * that cannot be drawn task-to-task roll up into one dashed package edge,
 * badged with how many real edges it stands for.
 */
export function layoutGraph(nodes: FlowNode[], deps: Dependency[], expandedWps: Set<string>): Graph {
  const index = buildIndex(nodes, deps);
  const wps = workPackages(nodes);
  const expanded = wps.filter((w) => expandedWps.has(w.id));
  const collapsed = wps.filter((w) => !expandedWps.has(w.id));

  const clusters: GraphCluster[] = [];
  const gnodes: GraphNode[] = [];
  const nodeRect = new Map<string, Rect>();

  let cy = 20;
  for (const wp of expanded) {
    const tasks = tasksOf(nodes, wp.id);
    const depth = new Map<string, number>();
    const own = new Set(tasks.map((t) => t.id));
    const depthOf = (id: string, seen = new Set<string>()): number => {
      if (depth.has(id)) return depth.get(id)!;
      if (seen.has(id)) return 0;
      seen.add(id);
      const ups = (index.blockers.get(id) ?? []).filter((b) => own.has(b));
      const d = ups.length ? Math.max(...ups.map((b) => depthOf(b, seen) + 1)) : 0;
      depth.set(id, d);
      return d;
    };
    for (const t of tasks) depthOf(t.id);

    const cols = new Map<number, FlowNode[]>();
    for (const t of tasks) {
      const d = depth.get(t.id) ?? 0;
      cols.set(d, [...(cols.get(d) ?? []), t]);
    }
    const colKeys = [...cols.keys()].sort((a, b) => a - b);
    const rows = Math.max(1, ...colKeys.map((k) => cols.get(k)!.length));
    const w = PAD * 2 + colKeys.length * NODE_W + Math.max(0, colKeys.length - 1) * COL_GAP;
    const h = HEADER + PAD + rows * NODE_H + Math.max(0, rows - 1) * ROW_GAP + PAD - 8;
    const hue = hueOf(nodes, wp.id);

    clusters.push({ wp, hue, counts: wpCounts(nodes, wp.id), x: CLUSTER_X, y: cy, w, h });

    colKeys.forEach((k, ci) => {
      cols.get(k)!.forEach((t, ri) => {
        const r: Rect = {
          x: CLUSTER_X + PAD + ci * (NODE_W + COL_GAP),
          y: cy + HEADER + ri * (NODE_H + ROW_GAP),
          w: NODE_W,
          h: NODE_H
        };
        nodeRect.set(t.id, r);
        gnodes.push({ ...r, node: t, hue, steps: stepRatio(nodes, t.id).label });
      });
    });

    cy += h + CLUSTER_GAP;
  }

  const boxes: GraphBox[] = collapsed.map((wp, i) => ({
    wp,
    hue: hueOf(nodes, wp.id),
    counts: wpCounts(nodes, wp.id),
    x: LEFT_COL,
    y: 40 + i * (BOX_H + BOX_GAP),
    w: BOX_W,
    h: BOX_H
  }));

  const clusterRect = new Map(clusters.map((c) => [c.wp.id, c as Rect]));
  const boxRect = new Map(boxes.map((b) => [b.wp.id, b as Rect]));

  /** Where does this node id appear on the canvas, and as what? */
  const resolve = (id: string): { key: string; rect: Rect; task: boolean } | null => {
    const n = index.byId.get(id);
    if (!n) return null;
    if (n.type === 'TASK') {
      const r = nodeRect.get(n.id);
      if (r) return { key: n.id, rect: r, task: true };
      const wpId = n.parentId ?? '';
      const b = boxRect.get(wpId);
      return b ? { key: wpId, rect: b, task: false } : null;
    }
    if (n.type === 'STEP') {
      const parent = n.parentId ? index.byId.get(n.parentId) : undefined;
      return parent ? resolve(parent.id) : null;
    }
    const c = clusterRect.get(n.id) ?? boxRect.get(n.id);
    return c ? { key: n.id, rect: c, task: false } : null;
  };

  const edges: GraphEdge[] = [];
  const rollups = new Map<string, { from: Rect; to: Rect; count: number; fromKey: string; toKey: string }>();

  for (const d of deps) {
    const a = resolve(d.blockerId);
    const b = resolve(d.blockedId);
    if (!a || !b || a.key === b.key) continue;
    if (a.task && b.task) {
      const blocker = index.byId.get(d.blockerId);
      const kind = blocker?.status === 'DONE' ? 'satisfied' : 'blocked';
      const p = path(a.rect, b.rect);
      edges.push({
        id: `${d.blockerId}->${d.blockedId}`,
        kind,
        from: d.blockerId,
        to: d.blockedId,
        ...p
      });
    } else {
      const key = `${a.key}=>${b.key}`;
      const cur = rollups.get(key);
      rollups.set(key, {
        from: a.rect,
        to: b.rect,
        count: (cur?.count ?? 0) + 1,
        fromKey: a.key,
        toKey: b.key
      });
    }
  }

  for (const [key, r] of rollups) {
    const p = path(r.from, r.to);
    edges.push({
      id: key,
      kind: 'rollup',
      from: r.fromKey,
      to: r.toKey,
      label: `via ${r.count} ${r.count === 1 ? 'task dep' : 'task deps'} ▸`,
      ...p
    });
  }

  const width = Math.max(
    CLUSTER_X + Math.max(0, ...clusters.map((c) => c.w)) + 60,
    LEFT_COL + BOX_W + 40
  );
  const height = Math.max(
    cy + 20,
    boxes.length ? boxes[boxes.length - 1].y + BOX_H + 40 : 0
  );
  return { width, height, clusters, nodes: gnodes, boxes, edges };
}

/** Cubic path between two rects, entering and leaving on facing edges. */
function path(a: Rect, b: Rect): { d: string; tx: number; ty: number; mx: number; my: number } {
  let x1: number, y1: number, x2: number, y2: number, horizontal: boolean;

  if (b.x >= a.x + a.w - 4) {
    x1 = a.x + a.w;
    y1 = a.y + a.h / 2;
    x2 = b.x;
    y2 = b.y + b.h / 2;
    horizontal = true;
  } else if (a.x >= b.x + b.w - 4) {
    x1 = a.x;
    y1 = a.y + a.h / 2;
    x2 = b.x + b.w;
    y2 = b.y + b.h / 2;
    horizontal = true;
  } else if (b.y >= a.y + a.h) {
    x1 = a.x + a.w / 2;
    y1 = a.y + a.h;
    x2 = b.x + b.w / 2;
    y2 = b.y;
    horizontal = false;
  } else {
    x1 = a.x + a.w / 2;
    y1 = a.y;
    x2 = b.x + b.w / 2;
    y2 = b.y + b.h;
    horizontal = false;
  }

  const d = horizontal
    ? `M${x1},${y1} C${x1 + Math.max(34, (x2 - x1) / 2)},${y1} ${x2 - Math.max(34, (x2 - x1) / 2)},${y2} ${x2},${y2}`
    : `M${x1},${y1} C${x1},${y1 + (y2 - y1) / 2} ${x2},${y2 - (y2 - y1) / 2} ${x2},${y2}`;

  return { d, tx: x2, ty: y2, mx: (x1 + x2) / 2, my: (y1 + y2) / 2 };
}

export function statusLabel(s: Status): string {
  return s;
}
