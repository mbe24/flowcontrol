export type NodeType = 'WORK_PACKAGE' | 'TASK' | 'STEP';
export type Status = 'READY' | 'BLOCKED' | 'DEFERRED' | 'DONE';

/** Explicit work-package lifecycle — an addition to datamodel.md v1.0. */
export type WPState = 'PLANNED' | 'ACTIVE' | 'DONE' | 'ARCHIVED';

/**
 * What the agent reported about a node's `condition`. fctrl NEVER runs a
 * condition itself — the agent runs it and reports, the human accepts.
 */
export type AgentResult = 'pass' | 'fail' | 'stale' | 'none';

/** The human's explicit override, independent of what the agent said. */
export type HumanVerdict = 'accepted' | 'rejected' | 'none';

export interface Verification {
  agent: AgentResult;
  /** Who reported it — an agent name, or '' when never reported. */
  agentName: string;
  agentWhen: string;
  human: HumanVerdict;
  humanWhen: string;
}

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
  /** Paragraphs. Markdown later; plain text for now. */
  description: string[];
  status: Status;
  condition?: string;
  /** STEP only — a few sentences of detail, shown on expand. */
  note?: string;
  /** WORK_PACKAGE only. */
  state?: WPState;
  /** TASK and WORK_PACKAGE only — steps show condition text but carry no flag. */
  verification?: Verification;
}

export interface Dependency {
  blockerId: string;
  blockedId: string;
}

export type ActivityKind = 'status' | 'verify' | 'edit' | 'comment';

export interface ActivityEntry {
  id: string;
  nodeId: string;
  kind: ActivityKind;
  /** Plain name. Agents get no special badge — authorship is just a byline. */
  author: string;
  when: string;
  text: string;
}

export const ALL_STATUSES: Status[] = ['READY', 'BLOCKED', 'DEFERRED', 'DONE'];

export const STATUS_VAR: Record<Status, string> = {
  READY: 'var(--ready)',
  BLOCKED: 'var(--blocked)',
  DEFERRED: 'var(--deferred)',
  DONE: 'var(--done)'
};

export const ACTIVITY_VAR: Record<ActivityKind, string> = {
  status: 'var(--accent)',
  verify: 'var(--ready)',
  edit: 'var(--fg2)',
  comment: 'var(--done)'
};

export function stepGlyph(s: Status): string {
  if (s === 'DONE') return '✓';
  if (s === 'READY') return '○';
  if (s === 'DEFERRED') return '⏸';
  return '·';
}

export const NO_VERIFICATION: Verification = {
  agent: 'none',
  agentName: '',
  agentWhen: '',
  human: 'none',
  humanWhen: ''
};

/** One resolved badge for a node's verification, human verdict winning. */
export interface VerifyBadge {
  glyph: string;
  color: string;
  bg: string;
  label: string;
  detail: string;
  /** True when the human has accepted — the override checkbox is ticked. */
  accepted: boolean;
}

export function verifyBadge(v: Verification | undefined): VerifyBadge {
  const ver = v ?? NO_VERIFICATION;
  const agentDetail = ver.agentName ? `${ver.agentName} · ${ver.agentWhen}` : '';

  if (ver.human === 'accepted') {
    const contradicts = ver.agent === 'fail';
    return {
      glyph: '✓',
      color: 'var(--ready)',
      bg: 'var(--ready-bg)',
      label: contradicts ? 'Accepted by you — agent reported failure' : 'Verified',
      detail: agentDetail ? `agent: ${ver.agent} · ${agentDetail}` : `you · ${ver.humanWhen}`,
      accepted: true
    };
  }
  if (ver.human === 'rejected') {
    return {
      glyph: '✕',
      color: 'var(--blocked)',
      bg: 'var(--blocked-bg)',
      label: 'Rejected by you',
      detail: agentDetail ? `agent: ${ver.agent} · ${agentDetail}` : `you · ${ver.humanWhen}`,
      accepted: false
    };
  }
  if (ver.agent === 'pass') {
    return { glyph: '✓', color: 'var(--ready)', bg: 'var(--ready-bg)', label: 'Verified by agent', detail: agentDetail, accepted: false };
  }
  if (ver.agent === 'fail') {
    return { glyph: '✕', color: 'var(--blocked)', bg: 'var(--blocked-bg)', label: 'Agent reported a failure', detail: agentDetail, accepted: false };
  }
  if (ver.agent === 'stale') {
    return { glyph: '◷', color: 'var(--fg2)', bg: 'var(--panel2)', label: 'Report is out of date', detail: agentDetail, accepted: false };
  }
  return { glyph: '–', color: 'var(--fg3)', bg: 'var(--panel2)', label: 'Not verified', detail: '', accepted: false };
}
