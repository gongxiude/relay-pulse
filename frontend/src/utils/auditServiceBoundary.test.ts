import { describe, expect, it } from 'vitest';

import { getAuditDataService } from './auditServiceBoundary';
import type { AuditChannelSnapshot } from '../types/audit';

function snapshot(service: string): Pick<AuditChannelSnapshot, 'service'> {
  return { service };
}

describe('getAuditDataService', () => {
  it('keeps anthropic as the audit API service', () => {
    expect(getAuditDataService(snapshot('anthropic'))).toBe('anthropic');
  });

  it('keeps openai as the audit API service', () => {
    expect(getAuditDataService(snapshot('openai'))).toBe('openai');
  });

  it('trims whitespace without converting to cc or cx', () => {
    expect(getAuditDataService(snapshot('  anthropic  '))).toBe('anthropic');
  });

  it('returns undefined for missing service', () => {
    expect(getAuditDataService(null)).toBeUndefined();
    expect(getAuditDataService({ service: '   ' })).toBeUndefined();
  });
});
