import { describe, expect, it } from 'vitest';

import { adaptAuditChannelsToMonitorData, inferAuditChannelType } from './auditChannelAdapter';
import type { ProcessedMonitorData } from '../types';

describe('inferAuditChannelType', () => {
  it('uses matched monitor O/R/M prefix before falling back to raw snapshot', () => {
    const type = inferAuditChannelType(
      {
        id: 1,
        newapi_channel_id: 80,
        snapshot_at: 1,
        provider: 'alan-官key直连',
        service: 'openai',
        channel: '80:alan-官key直连',
        model: 'gpt-5',
        enabled: true,
        raw: { Status: 1 },
      },
      {
        channel: 'O-api-main',
        channelName: 'api-main',
        auditChannelType: undefined,
      },
    );

    expect(type).toBe('official');
  });

  it('prefers backend snapshot channel type when it is present', () => {
    const type = inferAuditChannelType(
      {
        id: 1,
        newapi_channel_id: 82,
        snapshot_at: 1,
        provider: 'alan-号池',
        service: 'anthropic',
        channel: '82:alan-号池',
        model: 'claude-sonnet-4-6',
        enabled: true,
        channelType: 'unknown',
        raw: { Status: 1 },
      },
      {
        channel: 'O-api-main',
        channelName: 'api-main',
        auditChannelType: 'official',
      },
    );

    expect(type).toBe('unknown');
  });

  it('falls back to unknown when no existing type signal is present', () => {
    const type = inferAuditChannelType({
      id: 1,
      newapi_channel_id: 81,
      snapshot_at: 1,
      provider: 'alan-号池',
      service: 'anthropic',
      channel: '81:alan-号池',
      model: 'claude-sonnet-4-6',
      enabled: false,
      raw: { Status: 2 },
    });

    expect(type).toBe('unknown');
  });
});

describe('adaptAuditChannelsToMonitorData', () => {
  it('carries audit channel type label into processed rows', () => {
    const monitorIndex = new Map<string, ProcessedMonitorData>([
      [
        'alan-官key直连|cx|alan-官key直连',
        {
          id: 'm1',
          providerId: 'alan-官key直连',
          providerSlug: 'alan-官key直连',
          providerName: 'alan-官key直连',
          providerUrl: null,
          serviceType: 'cx',
          serviceName: 'cx',
          category: 'commercial',
          sponsor: '',
          sponsorUrl: null,
          priceMin: null,
          priceMax: null,
          listedDays: null,
          channel: 'O-api-main',
          channelName: 'api-main',
          board: 'hot',
          pinned: false,
          qualityScore: null,
          isMultiModel: false,
          history: [],
          currentStatus: 'AVAILABLE',
          uptime: 100,
        },
      ],
    ]);

    const rows = adaptAuditChannelsToMonitorData(
      [
        {
          id: 1,
          newapi_channel_id: 80,
          snapshot_at: 1,
          provider: 'alan-官key直连',
          service: 'openai',
          channel: '80:alan-官key直连',
          model: 'gpt-5',
          enabled: true,
          raw: { Status: 1 },
        },
      ],
      monitorIndex,
    );

    expect(rows[0].auditChannelType).toBe('official');
    expect(rows[0].auditChannelTypeLabel).toBe('官方直连');
  });
});
