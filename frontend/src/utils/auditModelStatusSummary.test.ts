import { describe, expect, it } from 'vitest';

import {
  aggregateAuditModelStatusForChannels,
  buildAuditChannelSummaryKey,
  buildAuditProviderSummaryKey,
} from './auditModelStatusSummary';
import type { AuditChannelSnapshot, AuditModelStatusItem } from '../types/audit';

function channel(overrides: Partial<AuditChannelSnapshot>): AuditChannelSnapshot {
  return {
    id: 1,
    newapi_channel_id: 80,
    snapshot_at: 1,
    provider: 'alan-官key直连',
    service: 'anthropic',
    channel: '80:alan-官key直连',
    model: 'claude-opus-4-8,claude-sonnet-4-5',
    enabled: true,
    raw: { Status: 1 },
    ...overrides,
  };
}

function item(overrides: Partial<AuditModelStatusItem>): AuditModelStatusItem {
  return {
    provider: 'alan-官key直连',
    service: 'anthropic',
    channel: '80:alan-官key直连',
    model: 'claude-opus-4-8',
    request_model: 'claude-opus-4-8',
    enabled: true,
    production: {
      source: 'production_logs',
      status: 'ok',
      total: 10,
      success: 9,
      error: 1,
      timeout: 1,
      success_rate: 90,
      p95: 2,
      p99: 3,
      updated_at: 100,
    },
    template_probe: {
      source: 'template_probe',
      status: 'available',
      window: '24h',
      total: 2,
      success: 1,
      degraded: 1,
      timeout: 0,
      no_response: 0,
      availability: 100,
    },
    quick_probe: {
      source: 'quick_probe',
      status: 'done',
      usable: true,
      baseline_mode: 'registered_baseline',
    },
    ...overrides,
  };
}

describe('aggregateAuditModelStatusForChannels', () => {
  it('aggregates real audit service data into channel and provider summaries', () => {
    const result = aggregateAuditModelStatusForChannels(
      [channel({})],
      [
        item({ model: 'claude-opus-4-8' }),
        item({
          model: 'claude-sonnet-4-5',
          production: {
            source: 'production_logs',
            status: 'ok',
            total: 5,
            success: 5,
            error: 0,
            timeout: 0,
            success_rate: 100,
            p95: 1,
            p99: 1,
          },
          template_probe: {
            source: 'template_probe',
            status: 'unavailable',
            window: '24h',
            total: 1,
            success: 0,
            degraded: 0,
            timeout: 1,
            no_response: 1,
            availability: 0,
          },
          quick_probe: {
            source: 'quick_probe',
            status: 'missing',
            usable: false,
          },
        }),
      ],
    );

    const channelSummary = result.byChannel.get(
      buildAuditChannelSummaryKey('alan-官key直连', 'anthropic', '80:alan-官key直连'),
    );
    expect(channelSummary?.viewService).toBe('cc');
    expect(channelSummary?.productionTotal).toBe(15);
    expect(channelSummary?.productionSuccess).toBe(14);
    expect(channelSummary?.productionSuccessRate).toBeCloseTo(93.333, 2);
    expect(channelSummary?.templateProbeTotal).toBe(3);
    expect(channelSummary?.templateProbeSuccess).toBe(2);
    expect(channelSummary?.templateProbeTimeout).toBe(1);
    expect(channelSummary?.templateProbeNoResponse).toBe(1);
    expect(channelSummary?.templateAvailability).toBeCloseTo(66.666, 2);
    expect(channelSummary?.quickProbeDone).toBe(1);
    expect(channelSummary?.quickProbeMissing).toBe(1);
    expect(channelSummary?.baselineCompared).toBe(1);

    const providerSummary = result.byProvider.get(
      buildAuditProviderSummaryKey('alan-官key直连', 'cc'),
    );
    expect(providerSummary?.totalModels).toBe(2);
    expect(providerSummary?.enabledModels).toBe(2);
    expect(providerSummary?.productionSuccessRate).toBeCloseTo(93.333, 2);
  });
});
