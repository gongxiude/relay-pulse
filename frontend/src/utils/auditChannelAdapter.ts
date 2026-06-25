import type { AuditChannelSnapshot } from '../types/audit';
import type { ProcessedMonitorData, StatusKey } from '../types';
import { normalizeChannelType, parseChannelType } from '../components/ChannelTypeIcon';
import { canonicalize } from './monitorDataProcessor';

const EMPTY_HISTORY: ProcessedMonitorData['history'] = [];

export type AuditSourceKey = 'recommended' | 'official' | 'reverse' | 'mixed' | 'unknown' | 'user';

export function extractAuditChannelName(channel: string): string {
  const text = channel.trim();
  const index = text.indexOf(':');
  if (index < 0) return text;
  return text.slice(index + 1).trim() || text;
}

export function inferAuditServiceType(snapshot: AuditChannelSnapshot): string {
  const haystack = [
    snapshot.provider,
    snapshot.service,
    snapshot.channel,
    snapshot.model,
    stringifyRaw(snapshot.raw),
  ].join(' ').toLowerCase();

  if (haystack.includes('anthropic') || haystack.includes('claude')) return 'cc';
  if (haystack.includes('gemini') || haystack.includes('google')) return 'gm';
  if (
    haystack.includes('openai') ||
    haystack.includes('gpt') ||
    haystack.includes('codex') ||
    haystack.includes('chatgpt') ||
    haystack.includes('o1') ||
    haystack.includes('o3') ||
    haystack.includes('o4')
  ) {
    return 'cx';
  }
  return 'cx';
}

export function buildAuditMonitorMatchKey(
  provider: string,
  serviceType: string,
  channelName: string,
): string {
  return `${canonicalize(provider)}|${serviceType.toLowerCase()}|${channelName.trim().toLowerCase()}`;
}

export function buildAuditStatusIndex(
  monitors: ProcessedMonitorData[],
): Map<string, ProcessedMonitorData> {
  const index = new Map<string, ProcessedMonitorData>();
  monitors.forEach((item) => {
    const channelName = (item.channelName || item.channel || '').trim().toLowerCase();
    if (!channelName) return;
    index.set(
      buildAuditMonitorMatchKey(item.providerId || item.providerSlug, item.serviceType, channelName),
      item,
    );
  });
  return index;
}

export function inferAuditChannelType(
  snapshot: AuditChannelSnapshot,
  matched?: Pick<ProcessedMonitorData, 'channel' | 'channelName' | 'auditChannelType'>,
): AuditSourceKey {
  const snapshotType = normalizeAuditSourceKey(snapshot.channelType);
  if (snapshotType) return snapshotType;

  const matchedType = normalizeAuditSourceKey(matched?.auditChannelType);
  if (matchedType) return matchedType;

  const matchedChannelType =
    parseChannelType(matched?.channel) ||
    parseChannelType(matched?.channelName) ||
    normalizeChannelType(matched?.channel) ||
    normalizeChannelType(matched?.channelName);
  if (matchedChannelType) return matchedChannelType;

  const channelType = parseChannelType(snapshot.channel);
  if (channelType) return channelType;

  const rawText = JSON.stringify(snapshot.raw || {}).toLowerCase();
  if (rawText.includes('recommend') || rawText.includes('董推')) return 'recommended';
  if (rawText.includes('user') || rawText.includes('submit')) return 'user';
  return 'unknown';
}

export function getAuditChannelTypeLabel(type: AuditSourceKey): string {
  if (type === 'official') return '官方直连';
  if (type === 'reverse') return '逆向';
  if (type === 'mixed') return '混合';
  if (type === 'recommended') return '董推';
  if (type === 'user') return '用户提交';
  return '未知';
}

export function adaptAuditChannelsToMonitorData(
  snapshots: AuditChannelSnapshot[],
  monitorIndex: Map<string, ProcessedMonitorData>,
): ProcessedMonitorData[] {
  return snapshots.map((snapshot) => {
    const serviceType = inferAuditServiceType(snapshot);
    const channelName = extractAuditChannelName(snapshot.channel);
    const providerId = canonicalize(snapshot.provider);
    const matched = monitorIndex.get(
      buildAuditMonitorMatchKey(providerId, serviceType, channelName.toLowerCase()),
    );
    const modelEntries = splitAuditModels(snapshot.model).map((model) => ({
      model,
      requestModel: model,
    }));
    const currentStatus: StatusKey = matched
      ? matched.currentStatus
      : snapshot.enabled
      ? 'MISSING'
      : 'UNAVAILABLE';
    const auditChannelType = inferAuditChannelType(snapshot, matched);

    return {
      id: `audit-${snapshot.newapi_channel_id}`,
      providerId,
      providerSlug: providerId,
      providerName: snapshot.provider,
      providerUrl: matched?.providerUrl ?? null,
      serviceType,
      serviceName: serviceType,
      category: matched?.category ?? 'commercial',
      sponsor: matched?.sponsor ?? '',
      sponsorUrl: matched?.sponsorUrl ?? null,
      sponsorLevel: matched?.sponsorLevel,
      annotations: matched?.annotations,
      priceMin: matched?.priceMin ?? null,
      priceMax: matched?.priceMax ?? null,
      listedDays: matched?.listedDays ?? null,
      channel: snapshot.channel,
      channelName,
      auditChannelType,
      auditChannelTypeLabel: snapshot.channelTypeLabel || getAuditChannelTypeLabel(auditChannelType),
      newApiStatusCode: getSnapshotStatusCode(snapshot),
      newApiStatusLabel: getSnapshotStatusLabel(snapshot),
      board: matched?.board ?? 'hot',
      coldReason: matched?.coldReason,
      probeUrl: matched?.probeUrl,
      templateName: matched?.templateName,
      intervalMs: matched?.intervalMs,
      slowLatencyMs: matched?.slowLatencyMs,
      pinned: false,
      qualityScore: matched?.qualityScore ?? null,
      isMultiModel: modelEntries.length > 1,
      layers: matched?.layers,
      modelEntries,
      history: matched?.history ?? EMPTY_HISTORY,
      currentStatus,
      uptime: matched?.uptime ?? -1,
      lastCheckTimestamp: matched?.lastCheckTimestamp,
      lastCheckLatency: matched?.lastCheckLatency,
    };
  });
}

function getSnapshotStatusCode(snapshot: AuditChannelSnapshot): number | null {
  const value = snapshot.raw?.Status;
  return typeof value === 'number' ? value : null;
}

function getSnapshotStatusLabel(snapshot: AuditChannelSnapshot): string {
  const code = getSnapshotStatusCode(snapshot);
  if (code === 1) return '已启用';
  if (code === null) return snapshot.enabled ? '已启用' : '已禁用';
  return `已禁用(S${code})`;
}

function normalizeAuditSourceKey(value?: string | null): AuditSourceKey | null {
  if (!value) return null;
  if (value === 'recommended') return 'recommended';
  if (value === 'official') return 'official';
  if (value === 'reverse') return 'reverse';
  if (value === 'mixed') return 'mixed';
  if (value === 'user') return 'user';
  if (value === 'unknown') return 'unknown';
  return null;
}

export function buildProviderDetailHref(
  item: Pick<ProcessedMonitorData, 'providerSlug' | 'serviceType' | 'channel'>,
  langPrefix?: string,
): string | null {
  if (!item.providerSlug) return null;
  const base = langPrefix ? `/${langPrefix}/p/${item.providerSlug}` : `/p/${item.providerSlug}`;
  const params = new URLSearchParams();
  if (item.serviceType) params.set('service', item.serviceType);
  if (item.channel) params.set('channel', item.channel);
  const query = params.toString();
  return query ? `${base}?${query}` : base;
}

function splitAuditModels(raw: string): string[] {
  return raw
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean);
}

function stringifyRaw(raw: AuditChannelSnapshot['raw']): string {
  if (!raw) return '';
  try {
    return JSON.stringify(raw);
  } catch {
    return '';
  }
}
