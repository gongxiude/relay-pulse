import type { AuditChannelSnapshot, AuditModelStatusItem } from '../types/audit';
import type { AuditDisplaySummary, StatusKey } from '../types';
import { canonicalize } from './monitorDataProcessor';
import { inferAuditServiceType } from './auditChannelAdapter';

export interface AuditSummaryIndexes {
  byChannel: Map<string, AuditDisplaySummary>;
  byProvider: Map<string, AuditDisplaySummary>;
  totals: AuditDisplaySummary;
}

export function buildAuditChannelSummaryKey(provider: string, service: string, channel: string): string {
  return `${canonicalize(provider)}|${service.trim().toLowerCase()}|${channel.trim().toLowerCase()}`;
}

export function buildAuditProviderSummaryKey(provider: string, viewService: string): string {
  return `${canonicalize(provider)}|${viewService.trim().toLowerCase()}`;
}

export function aggregateAuditModelStatusForChannels(
  channels: AuditChannelSnapshot[],
  items: AuditModelStatusItem[],
): AuditSummaryIndexes {
  const channelViewService = new Map<string, string>();
  channels.forEach((snapshot) => {
    channelViewService.set(
      buildAuditChannelSummaryKey(snapshot.provider, snapshot.service, snapshot.channel),
      inferAuditServiceType(snapshot),
    );
  });

  const byChannel = new Map<string, AuditDisplaySummary>();
  const byProvider = new Map<string, AuditDisplaySummary>();
  const totals = emptySummary('全部', 'all', 'all');

  items.forEach((item) => {
    const channelKey = buildAuditChannelSummaryKey(item.provider, item.service, item.channel);
    const viewService = channelViewService.get(channelKey) || inferViewServiceFromItem(item);
    const channelSummary = getOrCreateSummary(
      byChannel,
      channelKey,
      item.provider,
      item.service,
      viewService,
      item.channel,
    );
    addItem(channelSummary, item);

    const providerKey = buildAuditProviderSummaryKey(item.provider, viewService);
    const providerSummary = getOrCreateSummary(
      byProvider,
      providerKey,
      item.provider,
      item.service,
      viewService,
    );
    addItem(providerSummary, item);
    addItem(totals, item);
  });

  finalizeSummary(totals);
  byChannel.forEach(finalizeSummary);
  byProvider.forEach(finalizeSummary);
  return { byChannel, byProvider, totals };
}

function emptySummary(
  provider: string,
  service: string,
  viewService: string,
  channel?: string,
): AuditDisplaySummary {
  return {
    source: 'audit_model_status',
    provider,
    service,
    viewService,
    channel,
    totalModels: 0,
    enabledModels: 0,
    productionTotal: 0,
    productionSuccess: 0,
    productionSuccessRate: 0,
    templateProbeTotal: 0,
    templateProbeSuccess: 0,
    templateProbeTimeout: 0,
    templateProbeNoResponse: 0,
    templateAvailability: 0,
    quickProbeDone: 0,
    quickProbeFailed: 0,
    quickProbeMissing: 0,
    baselineCompared: 0,
  };
}

function getOrCreateSummary(
  map: Map<string, AuditDisplaySummary>,
  key: string,
  provider: string,
  service: string,
  viewService: string,
  channel?: string,
): AuditDisplaySummary {
  const existing = map.get(key);
  if (existing) return existing;
  const created = emptySummary(provider, service, viewService, channel);
  map.set(key, created);
  return created;
}

function addItem(summary: AuditDisplaySummary, item: AuditModelStatusItem): void {
  summary.totalModels += 1;
  if (item.enabled) summary.enabledModels += 1;
  summary.productionTotal += item.production.total || 0;
  summary.productionSuccess += item.production.success || 0;
  summary.templateProbeTotal += item.template_probe.total || 0;
  summary.templateProbeSuccess += (item.template_probe.success || 0) + (item.template_probe.degraded || 0);
  summary.templateProbeTimeout += item.template_probe.timeout || 0;
  summary.templateProbeNoResponse += item.template_probe.no_response || 0;

  if (item.quick_probe.status === 'done') {
    summary.quickProbeDone += 1;
  } else if (item.quick_probe.status === 'missing') {
    summary.quickProbeMissing += 1;
  } else {
    summary.quickProbeFailed += 1;
  }
  if (item.quick_probe.baseline_mode && item.quick_probe.baseline_mode !== 'candidate_only') {
    summary.baselineCompared += 1;
  }
}

function finalizeSummary(summary: AuditDisplaySummary): void {
  summary.productionSuccessRate =
    summary.productionTotal > 0 ? (summary.productionSuccess / summary.productionTotal) * 100 : 0;
  summary.templateAvailability =
    summary.templateProbeTotal > 0 ? (summary.templateProbeSuccess / summary.templateProbeTotal) * 100 : 0;
}

function inferViewServiceFromItem(item: AuditModelStatusItem): string {
  const probe = `${item.provider} ${item.service} ${item.channel} ${item.model}`.toLowerCase();
  if (probe.includes('anthropic') || probe.includes('claude')) return 'cc';
  if (probe.includes('gemini') || probe.includes('google')) return 'gm';
  return 'cx';
}

export function chooseAuditDisplayAvailability(summary?: AuditDisplaySummary | null): number {
  if (!summary) return -1;
  if (summary.productionTotal > 0) return roundAvailability(summary.productionSuccessRate);
  if (summary.templateProbeTotal > 0) return roundAvailability(summary.templateAvailability);
  return -1;
}

export function buildAuditDisplayHistory(summary?: AuditDisplaySummary | null): Array<{
  index: number;
  status: StatusKey;
  timestamp: string;
  timestampNum: number;
  latency: number;
  availability: number;
  statusCounts: { available: number; unavailable: number; degraded: number; missing: number };
}> {
  const availability = chooseAuditDisplayAvailability(summary);
  if (availability < 0) return [];
  const status: StatusKey = availability >= 99.5 ? 'AVAILABLE' : availability > 0 ? 'DEGRADED' : 'UNAVAILABLE';
  const now = Math.floor(Date.now() / 1000);
  return [2, 1, 0].map((offset) => ({
    index: 2 - offset,
    status,
    timestamp: new Date((now - offset * 3600) * 1000).toISOString(),
    timestampNum: now - offset * 3600,
    latency: 0,
    availability,
    statusCounts: {
      available: status === 'AVAILABLE' ? 1 : 0,
      unavailable: status === 'UNAVAILABLE' ? 1 : 0,
      degraded: status === 'DEGRADED' ? 1 : 0,
      missing: 0,
    },
  }));
}

function roundAvailability(value: number): number {
  return Math.round(value * 100) / 100;
}
