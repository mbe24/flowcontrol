import { describe, expect, it } from 'vitest';
import { NO_VERIFICATION, stepGlyph, verifyBadge } from './types';
import type { Verification } from './types';

const v = (over: Partial<Verification>): Verification => ({ ...NO_VERIFICATION, ...over });

describe('stepGlyph', () => {
  it('maps a status to its glyph', () => {
    expect(stepGlyph('DONE')).toBe('✓');
    expect(stepGlyph('READY')).toBe('○');
    expect(stepGlyph('DEFERRED')).toBe('⏸');
    expect(stepGlyph('BLOCKED')).toBe('·');
  });
});

describe('verifyBadge', () => {
  it('reports not verified when there is no report', () => {
    const b = verifyBadge(undefined);
    expect(b.accepted).toBe(false);
    expect(b.label).toBe('Not verified');
  });

  it('shows an agent pass', () => {
    const b = verifyBadge(v({ agent: 'pass', agentName: 'claude-code' }));
    expect(b.label).toBe('Verified by agent');
    expect(b.accepted).toBe(false);
  });

  it('shows a stale report', () => {
    const b = verifyBadge(v({ agent: 'stale' }));
    expect(b.label).toBe('Report is out of date');
  });

  it('an accepted override wins even over an agent failure', () => {
    const b = verifyBadge(v({ agent: 'fail', agentName: 'claude-code', human: 'accepted' }));
    expect(b.accepted).toBe(true);
    expect(b.label).toBe('Accepted by you — agent reported failure');
  });

  it('a rejection is shown distinctly', () => {
    const b = verifyBadge(v({ agent: 'pass', human: 'rejected' }));
    expect(b.label).toBe('Rejected by you');
    expect(b.accepted).toBe(false);
  });
});
