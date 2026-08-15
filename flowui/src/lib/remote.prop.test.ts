import fc from 'fast-check';
import { describe, expect, it } from 'vitest';
import { mapNode } from './remote';
import {
  EffectiveStatus as PbEffective,
  NodeKind as PbKind,
  type Node as PbNode
} from '@flow/api/flow/v1/flow_pb';

// A paragraph as the UI treats one: non-empty, trimmed, no internal blank lines.
const para = fc
  .string({ minLength: 1 })
  .map((s) => s.replace(/\s+/g, ' ').trim())
  .filter((s) => s.length > 0);

function pb(description: string): PbNode {
  return {
    id: 'x',
    projectId: 'p',
    parentId: '',
    kind: PbKind.TASK,
    title: 't',
    description,
    condition: '',
    note: '',
    reference: '',
    declaredStatus: 1,
    status: PbEffective.READY,
    wpState: 0,
    position: 0,
    createdAt: 0n,
    updatedAt: 0n
  } as unknown as PbNode;
}

describe('mapNode description split (property)', () => {
  it('round-trips clean paragraphs joined by blank lines', () => {
    fc.assert(
      fc.property(fc.array(para, { maxLength: 6 }), (paras) => {
        expect(mapNode(pb(paras.join('\n\n'))).description).toEqual(paras);
      })
    );
  });

  it('never yields an empty or untrimmed paragraph, for any input', () => {
    fc.assert(
      fc.property(fc.string(), (s) => {
        for (const p of mapNode(pb(s)).description) {
          expect(p.length).toBeGreaterThan(0);
          expect(p).toBe(p.trim());
        }
      })
    );
  });
});
