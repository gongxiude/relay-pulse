import { useMemo, useState } from 'react';
import { Helmet } from 'react-helmet-async';
import { Server } from 'lucide-react';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

import { Header } from './components/Header';
import { StatusTable } from './components/StatusTable';
import { useAuditChannels } from './hooks/useAuditChannels';
import { useAuditModelStatusSummary } from './hooks/useAuditModelStatus';
import { useMonitorData } from './hooks/useMonitorData';
import { useRpdiagScores } from './hooks/useRpdiagScores';
import { useSeoMeta } from './hooks/useSeoMeta';
import { adaptAuditChannelsToMonitorData, buildAuditStatusIndex } from './utils/auditChannelAdapter';
import type { ProcessedMonitorData, SortConfig } from './types';
import {
  aggregateAuditModelStatusForChannels,
  buildAuditChannelSummaryKey,
  buildAuditDisplayHistory,
  buildAuditProviderSummaryKey,
  chooseAuditDisplayAvailability,
} from './utils/auditModelStatusSummary';

const EMPTY_TOOLTIP_HANDLER = () => {};

function aggregateProviderRows(
  rows: ProcessedMonitorData[],
  providerSummaries = new Map<string, NonNullable<ProcessedMonitorData['auditSummary']>>(),
): ProcessedMonitorData[] {
  const groups = new Map<string, ProcessedMonitorData[]>();

  rows.forEach((row) => {
    const key = `${row.providerId}|${row.serviceType}`;
    const items = groups.get(key);
    if (items) {
      items.push(row);
    } else {
      groups.set(key, [row]);
    }
  });

  return Array.from(groups.values()).map((items) => {
    const sorted = [...items].sort((left, right) => {
      const leftEnabled = left.newApiStatusCode === 1 ? 1 : 0;
      const rightEnabled = right.newApiStatusCode === 1 ? 1 : 0;
      if (leftEnabled !== rightEnabled) return rightEnabled - leftEnabled;
      return (right.uptime ?? -1) - (left.uptime ?? -1);
    });
    const primary = sorted[0];
    const enabledCount = items.filter((item) => item.newApiStatusCode === 1).length;
    const uptimeValues = items.map((item) => item.uptime).filter((value) => value >= 0);
    const aggregateUptime = uptimeValues.length > 0
      ? uptimeValues.reduce((sum, value) => sum + value, 0) / uptimeValues.length
      : -1;
    const providerSummary = providerSummaries.get(
      buildAuditProviderSummaryKey(primary.providerName, primary.serviceType),
    );
    const auditAvailability = chooseAuditDisplayAvailability(providerSummary);

    return {
      ...primary,
      id: `provider-${primary.providerId}-${primary.serviceType}`,
      channel: primary.channel,
      channelName: items.length > 1 ? `${items.length} 条通道` : (primary.channelName || primary.channel),
      newApiStatusLabel: items.length > 1 ? `${enabledCount}/${items.length} 已启用` : primary.newApiStatusLabel,
      auditSummary: providerSummary ?? primary.auditSummary ?? null,
      uptime: auditAvailability >= 0 ? auditAvailability : aggregateUptime,
      history: primary.history.length > 0 ? primary.history : buildAuditDisplayHistory(providerSummary ?? primary.auditSummary),
      isMultiModel: true,
      modelEntries: [],
    };
  });
}

function inferAuditServiceForHome(row: ProcessedMonitorData): string {
  const text = `${row.serviceType} ${row.channel || ''} ${row.channelName || ''} ${row.providerName}`.toLowerCase();
  if (text.includes('claude') || text.includes('anthropic') || row.serviceType === 'cc') return 'anthropic';
  if (text.includes('gemini') || text.includes('google') || row.serviceType === 'gm') return 'gemini';
  return 'openai';
}

function sortRows(data: ProcessedMonitorData[], sortConfig: SortConfig): ProcessedMonitorData[] {
  const sorted = [...data];
  const factor = sortConfig.direction === 'asc' ? 1 : -1;

  sorted.sort((left, right) => {
    switch (sortConfig.key) {
      case 'provider':
        return factor * left.providerName.localeCompare(right.providerName, 'zh-CN');
      case 'service':
        return factor * left.serviceType.localeCompare(right.serviceType, 'en');
      case 'channel':
        return factor * (left.channelName || left.channel || '').localeCompare(right.channelName || right.channel || '', 'zh-CN');
      case 'status': {
        const leftStatus = left.newApiStatusCode ?? 99;
        const rightStatus = right.newApiStatusCode ?? 99;
        return factor * (leftStatus - rightStatus);
      }
      case 'uptime':
      default:
        return factor * ((left.uptime ?? -1) - (right.uptime ?? -1));
    }
  });

  return sorted;
}

function App() {
  const { t, i18n } = useTranslation();
  const location = useLocation();
  const seo = useSeoMeta({ pathname: location.pathname, language: i18n.language });
  const [sortConfig, setSortConfig] = useState<SortConfig>({ key: 'uptime', direction: 'desc' });

  const { scores: rpdiagScores, loaded: rpdiagScoresLoaded } = useRpdiagScores();
  const { channels: auditChannels, loading: auditChannelsLoading, error: auditChannelsError } = useAuditChannels();
  const {
    items: auditStatusItems,
    meta: auditStatusMeta,
    loading: auditStatusLoading,
    error: auditStatusError,
  } = useAuditModelStatusSummary({ window: '24h' });
  const {
    rawData,
    loading: monitorLoading,
    error: monitorError,
    slowLatencyMs,
    enableAnnotations,
    rpdiagEnabled,
  } = useMonitorData({
    timeRange: '90m',
    board: 'all',
    filterService: [],
    filterProvider: [],
    filterChannel: [],
    filterCategory: [],
    sortConfig: { key: 'uptime', direction: 'desc' },
    isInitialSort: false,
    autoRefresh: true,
    rpdiagScores,
    rpdiagScoresLoaded,
  });

  const rows = useMemo(() => {
    const monitorIndex = buildAuditStatusIndex(rawData);
    return adaptAuditChannelsToMonitorData(auditChannels, monitorIndex);
  }, [auditChannels, rawData]);

  const auditSummaryIndexes = useMemo(() => {
    return aggregateAuditModelStatusForChannels(auditChannels, auditStatusItems);
  }, [auditChannels, auditStatusItems]);

  const rowsWithAuditSummary = useMemo(() => {
    return rows.map((row) => {
      const summary = auditSummaryIndexes.byChannel.get(
        buildAuditChannelSummaryKey(row.providerName, row.auditSummary?.service || inferAuditServiceForHome(row), row.channel || ''),
      );
      const availability = chooseAuditDisplayAvailability(summary);
      if (!summary || availability < 0) return row;
      return {
        ...row,
        auditSummary: summary,
        uptime: availability,
        history: row.history.length > 0 ? row.history : buildAuditDisplayHistory(summary),
        currentStatus:
          availability >= 99.5 ? 'AVAILABLE' : availability > 0 ? 'DEGRADED' : 'UNAVAILABLE',
      } satisfies ProcessedMonitorData;
    });
  }, [rows, auditSummaryIndexes]);

  const providerRows = useMemo(
    () => aggregateProviderRows(rowsWithAuditSummary, auditSummaryIndexes.byProvider),
    [rowsWithAuditSummary, auditSummaryIndexes],
  );
  const sortedRows = useMemo(() => sortRows(providerRows, sortConfig), [providerRows, sortConfig]);

  const headerStats = useMemo(() => {
    const total = providerRows.length;
    const healthy = providerRows.filter((row) => row.newApiStatusCode === 1).length;
    return {
      total,
      healthy,
      issues: Math.max(0, total - healthy),
    };
  }, [providerRows]);

  const effectiveError = auditChannelsError || monitorError || auditStatusError;
  const loading = auditChannelsLoading || monitorLoading || auditStatusLoading;
  const auditSummary = auditStatusMeta?.summary;

  const handleSort = (key: string) => {
    setSortConfig((current) => {
      if (current.key === key) {
        return {
          key,
          direction: current.direction === 'asc' ? 'desc' : 'asc',
        };
      }
      return {
        key,
        direction: key === 'provider' || key === 'service' || key === 'channel' ? 'asc' : 'desc',
      };
    });
  };

  return (
    <>
      <Helmet>
        <html lang={seo.htmlLang} />
        <title>RelayPulse</title>
        <meta
          name="description"
          content="RelayPulse 首页基于 new-api 同步的真实服务商渠道数据，展示当前状态、可用率和可用率趋势。"
        />
      </Helmet>

      <div className="min-h-screen bg-page text-primary font-sans selection-accent">
        <div className="max-w-7xl mx-auto px-4 py-6 sm:px-6 lg:px-8">
          <Header stats={headerStats} />

          <section className="mb-5 rounded-2xl border border-default/70 bg-surface/55 px-5 py-5">
            <h1 className="text-3xl font-bold tracking-tight text-primary">服务商列表</h1>
            <p className="mt-3 text-secondary text-base leading-relaxed">
              首页只保留服务商入口。数据全部来自 `new-api` 同步快照，表格只展示当前状态、可用率和可用率趋势。点击服务商后进入详情页，查看该通道下每个模型的状态。
            </p>
            <div className="mt-4 flex flex-wrap items-center gap-3 text-sm text-secondary">
              <span>服务商 {providerRows.length}</span>
              <span>同步通道 {rows.length}</span>
              <span>生产日志样本 {auditSummary?.production_total ?? 0}</span>
              <span>模板样本 {auditSummary?.template_probe_total ?? 0}</span>
              <span>Baseline 对比 {auditSummary?.baseline_compared ?? 0}</span>
            </div>
          </section>

          <main className="overflow-hidden rounded-2xl border border-default/70 bg-surface/55 shadow-xl backdrop-blur-sm">
            {effectiveError ? (
              <div className="flex min-h-[320px] flex-col items-center justify-center px-6 py-16 text-danger">
                <Server size={48} className="mb-4 opacity-30" />
                <p className="text-lg">{t('common.error', { message: effectiveError })}</p>
              </div>
            ) : loading && providerRows.length === 0 ? (
              <div className="flex min-h-[320px] flex-col items-center justify-center gap-4 px-6 py-16 text-muted">
                <div className="h-12 w-12 animate-spin rounded-full border-4 border-accent/20" style={{ borderTopColor: 'hsl(var(--accent))' }} />
                <p>{t('common.loading')}</p>
              </div>
            ) : providerRows.length === 0 ? (
              <div className="flex min-h-[320px] flex-col items-center justify-center px-6 py-16 text-muted">
                <Server size={48} className="mb-4 opacity-30" />
                <p className="text-lg">{t('common.noData')}</p>
              </div>
            ) : (
              <StatusTable
                data={sortedRows}
                sortConfig={sortConfig}
                isInitialSort={false}
                timeRange="90m"
                slowLatencyMs={slowLatencyMs}
                enableAnnotations={enableAnnotations}
                showCategoryTag={false}
                showSponsor={false}
                showModel={false}
                showListedDays={false}
                showLastCheck={false}
                showQuality={false}
                isFavorite={() => false}
                onToggleFavorite={() => {}}
                onSort={handleSort}
                onBlockHover={EMPTY_TOOLTIP_HANDLER}
                onBlockLeave={EMPTY_TOOLTIP_HANDLER}
                rpdiagScores={rpdiagScores}
                rpdiagScoresLoaded={rpdiagScoresLoaded}
                rpdiagEnabled={rpdiagEnabled}
                hidePriceColumn
              />
            )}
          </main>
        </div>
      </div>
    </>
  );
}

export default App;
