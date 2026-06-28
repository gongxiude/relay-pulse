import { describe, expect, it } from 'vitest';

import { buildAuditModelStatusQuery } from './useAuditModelStatus';

describe('buildAuditModelStatusQuery', () => {
  it('can build a global summary query without provider/service/channel', () => {
    expect(buildAuditModelStatusQuery({ window: '24h' })).toBe('window=24h');
  });

  it('keeps filtered detail query parameters when provided', () => {
    expect(
      buildAuditModelStatusQuery({
        provider: 'alan-官key直连',
        service: 'anthropic',
        channel: '80:alan-官key直连',
        window: '24h',
      }),
    ).toBe(
      'provider=alan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&service=anthropic&channel=80%3Aalan-%E5%AE%98key%E7%9B%B4%E8%BF%9E&window=24h',
    );
  });
});
