import { describe, it, expect } from 'vitest';

import { buildRpdiagKey, lookupRpdiagScore } from './useRpdiagScores';
import type { RpdiagScore, RpdiagScoresResponse } from '../types/monitor';

describe('buildRpdiagKey', () => {
  it('joins by raw channel_name without stripping the source prefix', () => {
    // "O-Max" keeps its prefix → "o-max" (no strip to "max").
    expect(buildRpdiagKey('SAIAi', 'cc', 'O-Max')).toBe('saiai|cc|o-max');
    // The two codex tiers of one provider stay distinct instead of both
    // collapsing to "...|cx".
    expect(buildRpdiagKey('right.codes', 'cx', 'o-cx')).toBe('right.codes|cx|o-cx');
    expect(buildRpdiagKey('right.codes', 'cx', 'u-cx')).toBe('right.codes|cx|u-cx');
  });

  it('trims and lower-cases each segment', () => {
    expect(buildRpdiagKey('  SaiAI ', ' CC ', '  O-Max ')).toBe('saiai|cc|o-max');
  });
});

describe('lookupRpdiagScore', () => {
  const score = (max: number): RpdiagScore => ({ max_score: max }) as RpdiagScore;
  const scores: RpdiagScoresResponse = {
    'right.codes|cx|o-cx': score(95),
    'right.codes|cx|u-cx': score(90),
  };

  it('resolves o-cx and u-cx codex tiers to separate scores', () => {
    expect(lookupRpdiagScore(scores, 'right.codes', 'cx', 'o-cx')?.max_score).toBe(95);
    expect(lookupRpdiagScore(scores, 'right.codes', 'cx', 'u-cx')?.max_score).toBe(90);
  });

  it('returns undefined for a stripped key, missing args, or missing map', () => {
    // The old prefix-stripped key must no longer resolve.
    expect(lookupRpdiagScore(scores, 'right.codes', 'cx', 'cx')).toBeUndefined();
    expect(lookupRpdiagScore(scores, undefined, 'cx', 'o-cx')).toBeUndefined();
    expect(lookupRpdiagScore(undefined, 'right.codes', 'cx', 'o-cx')).toBeUndefined();
  });
});
