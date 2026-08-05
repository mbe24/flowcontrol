export type NodeType = 'WORK_PACKAGE' | 'TASK' | 'STEP';
export type Status = 'READY' | 'BLOCKED' | 'DEFERRED' | 'DONE';

/** Explicit work-package lifecycle — an addition to datamodel.md v1.0. */
export type WPState = 'PLANNED' | 'ACTIVE' | 'DONE' | 'ARCHIVED';

/** Cached outcome of running a node's `condition` — also an addition. */
export type VerifyResult = 'pass' | 'fail' | 'stale' | 'none';

export interface Project {
  id: string;
  name: string;
  description: string;
  createdAt: number;
}

export interface FlowNode {
  id: string;
  projectId: string;
  parentId: string | null;
  type: NodeType;
  title: string;
  description?: string;
  status: Status;
  condition?: string;
  /** WORK_PACKAGE only. */
  state?: WPState;
  lastResult: VerifyResult;
  lastRun?: string;
}

export interface Dependency {
  blockerId: string;
  blockedId: string;
}

export const ALL_STATUSES: Status[] = ['READY', 'BLOCKED', 'DEFERRED', 'DONE'];

export const STATUS_VAR: Record<Status, string> = {
  READY: 'var(--ready)',
  BLOCKED: 'var(--blocked)',
  DEFERRED: 'var(--deferred)',
  DONE: 'var(--done)',
};

export function stepGlyph(s: Status): string {
  if (s === 'DONE') return '✓';
  if (s === 'READY') return '○';
  if (s === 'DEFERRED') return '⏸';
  return '·';
}

export function verifyGlyph(r: VerifyResult): { glyph: string; color: string } {
  if (r === 'pass') return { glyph: '✓', color: 'var(--ready)' };
  if (r === 'fail') return { glyph: '✕', color: 'var(--blocked)' };
  if (r === 'stale') return { glyph: '◷', color: 'var(--fg2)' };
  return { glyph: '–', color: 'var(--fg3)' };
}

export function verifyText(r: VerifyResult): string {
  if (r === 'pass') return 'Condition passed';
  if (r === 'fail') return 'Condition failed';
  if (r === 'stale') return 'Last run predates recent commits — re-verify';
  return 'Never verified';
}
