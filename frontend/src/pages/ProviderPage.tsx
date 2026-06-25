import { useEffect, useMemo } from 'react';
import { useLocation, useParams, useSearchParams } from 'react-router-dom';
import { Helmet } from 'react-helmet-async';
import { CircleHelp, Sparkles } from 'lucide-react';

import { Header } from '../components/Header';
import { ChannelTypeIcon, parseChannelType } from '../components/ChannelTypeIcon';
import { useAuditChannels } from '../hooks/useAuditChannels';
import { useAuditDiagnosticLatest } from '../hooks/useAuditDiagnosticLatest';
import { useAuditSyncStatus } from '../hooks/useAuditSyncStatus';
import { useMonitorData } from '../hooks/useMonitorData';
import { useRpdiagScores, lookupRpdiagScore } from '../hooks/useRpdiagScores';
import { useSeoMeta } from '../hooks/useSeoMeta';
import type { ProcessedMonitorData } from '../types';
import type { AuditChannelSnapshot } from '../types/audit';
import type { RpdiagModelScore, RpdiagScoreTrend } from '../types/monitor';
import {
  buildAuditMonitorMatchKey,
  buildAuditStatusIndex,
  extractAuditChannelName,
  inferAuditServiceType,
} from '../utils/auditChannelAdapter';
import { canonicalize } from '../utils/monitorDataProcessor';

type ServiceTab = 'cc' | 'cx';
type SourceKey = 'all' | 'recommended' | 'official' | 'reverse' | 'mixed' | 'unknown' | 'user';

interface ModelDetailRow {
  id: string;
  modelName: string;
  finalScore: number | null;
  fingerprintScore: number | null;
  trend?: RpdiagScoreTrend | null;
  testsCount: number | null;
  uptime: number | null;
  avgLatencyMs: number | null;
  p95LatencyMs: number | null;
  ttftMs: number | null;
  enabled: boolean;
  latestRunId?: string | null;
  compareUrl?: string | null;
  latestMethodologyVersion?: string | null;
  latestAttemptStatus?: string | null;
  latestAttemptReason?: string | null;
  latestAttemptCreatedAt?: number | null;
}

const SERVICE_TAB_LABELS: Record<ServiceTab, string> = {
  cc: 'Claude Code',
  cx: 'Codex',
};

const SOURCE_META: Record<Exclude<SourceKey, 'all'>, { label: string; icon: React.ReactNode }> = {
  recommended: { label: '董推', icon: <Sparkles size={14} className="text-cyan-300" /> },
  official: { label: '服务商自报官方通道 (O-)', icon: <ChannelTypeIcon channel="O-demo" /> },
  reverse: { label: '逆向 (R-)', icon: <ChannelTypeIcon channel="R-demo" /> },
  mixed: { label: '混合 (M-)', icon: <ChannelTypeIcon channel="M-demo" /> },
  unknown: { label: '未知', icon: <ChannelTypeIcon channel="X-demo" /> },
  user: { label: '用户提交 (U-)', icon: <CircleHelp size={14} className="text-slate-300" /> },
};

export default function ProviderPage() {
  const { provider } = useParams<{ provider: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const location = useLocation();
  const seo = useSeoMeta({ pathname: location.pathname, language: 'zh-CN' });
  const langPrefix = useMemo(() => {
    const match = location.pathname.match(/^\/(en|ru|ja)(\/|$)/);
    return match ? `/${match[1]}` : '';
  }, [location.pathname]);

  const normalizedProvider = canonicalize(provider);
  const { channels: auditChannels, loading: auditLoading, error: auditError } = useAuditChannels();
  const { scores: rpdiagScores, loaded: rpdiagLoaded } = useRpdiagScores();
  const { data: syncStatus } = useAuditSyncStatus();
  const {
    rawData,
    loading: monitorLoading,
    error: monitorError,
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
    rpdiagScoresLoaded: rpdiagLoaded,
  });

  const providerSnapshots = useMemo(() => {
    return auditChannels.filter((snapshot) => canonicalize(snapshot.provider) === normalizedProvider);
  }, [auditChannels, normalizedProvider]);

  const providerExists = providerSnapshots.length > 0;
  const providerDisplayName = providerSnapshots[0]?.provider || provider || '';
  const providerServiceTabs = useMemo<ServiceTab[]>(() => {
    const set = new Set<ServiceTab>();
    providerSnapshots.forEach((snapshot) => {
      const serviceType = inferAuditServiceType(snapshot);
      if (serviceType === 'cc' || serviceType === 'cx') {
        set.add(serviceType);
      }
    });
    return Array.from(set);
  }, [providerSnapshots]);

  const selectedService = useMemo<ServiceTab>(() => {
    const value = searchParams.get('service')?.toLowerCase();
    if ((value === 'cc' || value === 'cx') && providerServiceTabs.includes(value)) {
      return value;
    }
    return providerServiceTabs[0] || 'cc';
  }, [searchParams, providerServiceTabs]);

  const serviceSnapshots = useMemo(() => {
    return providerSnapshots.filter((snapshot) => inferAuditServiceType(snapshot) === selectedService);
  }, [providerSnapshots, selectedService]);

  const sourceOptions = useMemo(() => {
    const keys = new Set<SourceKey>();
    serviceSnapshots.forEach((snapshot) => keys.add(inferSourceKey(snapshot)));
    return ['all', ...Array.from(keys).sort()] as SourceKey[];
  }, [serviceSnapshots]);

  const selectedSource = useMemo<SourceKey>(() => {
    const value = (searchParams.get('source') || 'all') as SourceKey;
    return sourceOptions.includes(value) ? value : 'all';
  }, [searchParams, sourceOptions]);

  const sourceFilteredSnapshots = useMemo(() => {
    if (selectedSource === 'all') return serviceSnapshots;
    return serviceSnapshots.filter((snapshot) => inferSourceKey(snapshot) === selectedSource);
  }, [serviceSnapshots, selectedSource]);

  const channelOptions = useMemo(() => {
    return sourceFilteredSnapshots.map((snapshot) => ({
      value: snapshot.channel,
      label: extractAuditChannelName(snapshot.channel),
    }));
  }, [sourceFilteredSnapshots]);

  const selectedChannel = useMemo(() => {
    const value = searchParams.get('channel');
    if (value && channelOptions.some((option) => option.value === value)) {
      return value;
    }
    return channelOptions[0]?.value || '';
  }, [searchParams, channelOptions]);

  const currentSnapshot = useMemo(() => {
    return sourceFilteredSnapshots.find((snapshot) => snapshot.channel === selectedChannel) || null;
  }, [sourceFilteredSnapshots, selectedChannel]);

  const monitorIndex = useMemo(() => buildAuditStatusIndex(rawData), [rawData]);
  const matchedMonitor = useMemo<ProcessedMonitorData | undefined>(() => {
    if (!currentSnapshot) return undefined;
    const serviceType = inferAuditServiceType(currentSnapshot);
    const channelName = extractAuditChannelName(currentSnapshot.channel).toLowerCase();
    return monitorIndex.get(
      buildAuditMonitorMatchKey(currentSnapshot.provider, serviceType, channelName),
    );
  }, [currentSnapshot, monitorIndex]);

  const currentRpdiag = useMemo(() => {
    if (!currentSnapshot) return undefined;
    const serviceType = inferAuditServiceType(currentSnapshot);
    const channelName = extractAuditChannelName(currentSnapshot.channel);
    return lookupRpdiagScore(
      rpdiagScores,
      matchedMonitor?.providerId || canonicalize(currentSnapshot.provider),
      serviceType,
      matchedMonitor?.channelName || channelName,
    );
  }, [currentSnapshot, rpdiagScores, matchedMonitor]);

  const {
    items: latestDiagnostics,
    loading: latestDiagnosticsLoading,
    error: latestDiagnosticsError,
  } = useAuditDiagnosticLatest({
    provider: currentSnapshot?.provider,
    service: currentSnapshot ? inferAuditServiceType(currentSnapshot) : undefined,
    channel: currentSnapshot?.channel,
    includeFiltered: true,
    limit: 10,
  });

  const latestDiagnosticMap = useMemo(() => {
    const map = new Map<string, typeof latestDiagnostics[number]>();
    latestDiagnostics.forEach((item) => {
      if (!item.usable) return;
      const key = normalizeModelKey(item.run.model);
      if (key && !map.has(key)) {
        map.set(key, item);
      }
    });
    return map;
  }, [latestDiagnostics]);

  const latestAttemptMap = useMemo(() => {
    const map = new Map<string, typeof latestDiagnostics[number]>();
    latestDiagnostics.forEach((item) => {
      const key = normalizeModelKey(item.run.model);
      if (key && !map.has(key)) {
        map.set(key, item);
      }
    });
    return map;
  }, [latestDiagnostics]);

  const modelOptions = useMemo(() => {
    if (!currentSnapshot) return [];
    return splitModels(currentSnapshot.model);
  }, [currentSnapshot]);

  const selectedModel = useMemo(() => {
    const value = searchParams.get('model') || 'all';
    return value === 'all' || modelOptions.includes(value) ? value : 'all';
  }, [searchParams, modelOptions]);

  const currentSourceKey = useMemo<SourceKey>(() => {
    if (!currentSnapshot) return 'unknown';
    return inferSourceKey(currentSnapshot);
  }, [currentSnapshot]);

  const currentSourceMeta = useMemo(() => {
    if (currentSourceKey === 'all') return null;
    return SOURCE_META[currentSourceKey as Exclude<SourceKey, 'all'>];
  }, [currentSourceKey]);

  const currentServiceGroup = currentSnapshot?.service || '--';
  const currentServiceViewLabel = SERVICE_TAB_LABELS[selectedService];

  useEffect(() => {
    if (!providerExists) return;
    const next = new URLSearchParams(searchParams);
    let changed = false;

    if (searchParams.get('service') !== selectedService) {
      next.set('service', selectedService);
      changed = true;
    }
    if (selectedSource === 'all') {
      if (next.has('source')) {
        next.delete('source');
        changed = true;
      }
    } else if (searchParams.get('source') !== selectedSource) {
      next.set('source', selectedSource);
      changed = true;
    }
    if (selectedChannel) {
      if (searchParams.get('channel') !== selectedChannel) {
        next.set('channel', selectedChannel);
        changed = true;
      }
    } else if (next.has('channel')) {
      next.delete('channel');
      changed = true;
    }
    if (selectedModel === 'all') {
      if (next.has('model')) {
        next.delete('model');
        changed = true;
      }
    } else if (searchParams.get('model') !== selectedModel) {
      next.set('model', selectedModel);
      changed = true;
    }

    if (changed) {
      setSearchParams(next, { replace: true });
    }
  }, [
    providerExists,
    searchParams,
    selectedService,
    selectedSource,
    selectedChannel,
    selectedModel,
    setSearchParams,
  ]);

  const modelRows = useMemo<ModelDetailRow[]>(() => {
    if (!currentSnapshot) return [];

    const baseModels = splitModels(currentSnapshot.model);
    const rpdiagModels = currentRpdiag?.models ?? [];
    const modelMap = new Map<string, RpdiagModelScore>();

    rpdiagModels.forEach((model) => {
      const key = normalizeModelKey(model.model || model.model_key);
      if (key) modelMap.set(key, model);
    });

    const mergedModels = [...baseModels];
    rpdiagModels.forEach((model) => {
      const name = model.model || model.model_key;
      if (!name) return;
      if (!mergedModels.some((item) => normalizeModelKey(item) === normalizeModelKey(name))) {
        mergedModels.push(name);
      }
    });

    const filtered = mergedModels
      .filter((model) => selectedModel === 'all' || model === selectedModel)
      .map((modelName) => {
        const rpModel = modelMap.get(normalizeModelKey(modelName));
        const latestDiagnostic = latestDiagnosticMap.get(normalizeModelKey(modelName));
        const latestAttempt = latestAttemptMap.get(normalizeModelKey(modelName));
        const latestScore = latestDiagnostic?.score?.overall_score ?? null;
        const latestAttemptRunId = latestAttempt?.run.run_id ?? null;
        const localCompareUrl = latestAttemptRunId
          ? `${langPrefix}/detect/compare/${latestAttemptRunId}`
          : null;
        return {
          id: `${currentSnapshot.newapi_channel_id}-${modelName}`,
          modelName,
          finalScore: latestScore ?? rpModel?.score ?? null,
          fingerprintScore: latestScore ?? rpModel?.score ?? currentRpdiag?.max_score ?? null,
          trend: rpModel?.trend,
          testsCount: pickTestsCount(rpModel?.trend),
          uptime: matchedMonitor?.uptime ?? null,
          avgLatencyMs: computeAverageLatency(matchedMonitor?.history),
          p95LatencyMs: computeP95Latency(matchedMonitor?.history),
          ttftMs: null,
          enabled: currentSnapshot.enabled,
          latestRunId: latestAttemptRunId,
          compareUrl: localCompareUrl,
          latestMethodologyVersion: latestDiagnostic?.score?.methodology_version ?? latestDiagnostic?.run.methodology_version ?? null,
          latestAttemptStatus: latestAttempt?.usable
            ? (latestAttempt?.run.run_status ?? latestAttempt?.run.status ?? null)
            : (latestAttempt?.filter_reason ?? latestAttempt?.run.run_status ?? latestAttempt?.run.status ?? null),
          latestAttemptReason: latestAttempt?.run.run_status_reason ?? latestAttempt?.filter_reason ?? null,
          latestAttemptCreatedAt: latestAttempt?.run.created_at ?? null,
        } satisfies ModelDetailRow;
      })

    return filtered.map((row, index) => ({
      ...row,
      id: `${row.id}-${index}`,
    }));
  }, [currentSnapshot, currentRpdiag, langPrefix, latestAttemptMap, latestDiagnosticMap, matchedMonitor, selectedModel]);

  const headerStats = useMemo(() => {
    const total = modelRows.length;
    const healthy = modelRows.filter((row) => row.enabled && (row.uptime ?? 0) > 0).length;
    return {
      total,
      healthy,
      issues: Math.max(0, total - healthy),
    };
  }, [modelRows]);

  const showProbeWarning = useMemo(() => {
    if (!currentSnapshot || latestDiagnosticsLoading) return false;
    if (latestDiagnostics.some((item) => item.usable)) return false;
    return Boolean(syncStatus?.probe_runtime?.warning);
  }, [currentSnapshot, latestDiagnostics, latestDiagnosticsLoading, syncStatus]);

  const diagnosticSummary = useMemo(() => {
    let usable = 0;
    let failedAuth = 0;
    let failedRequest = 0;
    let pending = 0;
    latestDiagnostics.forEach((item) => {
      if (item.usable) {
        usable += 1;
        return;
      }
      const status = (item.filter_reason || item.run.run_status || item.run.status || '').toLowerCase();
      if (status === 'failed_auth') failedAuth += 1;
      else if (status === 'failed_request') failedRequest += 1;
      else pending += 1;
    });
    return { usable, failedAuth, failedRequest, pending };
  }, [latestDiagnostics]);

  const pageTitle = providerDisplayName ? `${providerDisplayName} - 服务商详情` : '服务商详情';

  if (!auditLoading && !providerExists && !auditError) {
    return (
      <>
        <Helmet>
          <html lang={seo.htmlLang} />
          <title>404 - 服务商未找到</title>
          <meta name="robots" content="noindex, nofollow" />
        </Helmet>
        <div className="min-h-screen bg-page text-primary flex items-center justify-center px-4">
          <div className="rounded-2xl border border-default bg-surface/60 px-8 py-10 text-center max-w-lg w-full">
            <div className="text-5xl font-bold mb-3">404</div>
            <p className="text-secondary">未找到对应的服务商详情页。</p>
          </div>
        </div>
      </>
    );
  }

  return (
    <>
      <Helmet>
        <html lang={seo.htmlLang} />
        <title>{pageTitle}</title>
        <meta
          name="description"
          content={`${providerDisplayName} 的模型级质量与可用率详情页，按模型展示最终质量分、机器指纹分、趋势与监控补充指标。`}
        />
      </Helmet>

      <div className="min-h-screen bg-page text-primary font-sans selection-accent">
        <div className="max-w-7xl mx-auto px-4 py-6 sm:px-6 lg:px-8">
          <Header stats={headerStats} />

          <section className="mb-5 rounded-2xl border border-default/70 bg-surface/55 px-5 py-5">
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-3xl font-bold tracking-tight text-primary">{providerDisplayName || '服务商详情'}</h1>
              {currentSourceMeta ? (
                <span className="inline-flex items-center gap-1.5 rounded-full border border-default/70 bg-surface/70 px-3 py-1 text-sm text-secondary">
                  {currentSourceMeta.icon}
                  {currentSnapshot?.channelTypeLabel || currentSourceMeta.label}
                </span>
              ) : null}
              {currentSnapshot ? <SnapshotStatusBadge snapshot={currentSnapshot} /> : null}
            </div>
            <p className="mt-3 text-secondary text-base leading-relaxed">
              当前页只使用 `new-api` 同步的真实渠道快照。先选中当前要看的通道，再按模型查看启停状态、最近检测状态和可用率趋势。
            </p>
            <div className="mt-4 flex flex-wrap items-center gap-3">
              <span className="text-sm text-secondary">服务视图</span>
              {providerServiceTabs.length > 1 ? (
                <div className="inline-flex rounded-xl border border-default/70 bg-surface/70 p-1">
                  {(['cc', 'cx'] as ServiceTab[]).map((tab) => {
                    const active = selectedService === tab;
                    const disabled = !providerServiceTabs.includes(tab);
                    return (
                      <button
                        key={tab}
                        type="button"
                        disabled={disabled}
                        onClick={() => updateParam(setSearchParams, searchParams, { service: tab, channel: undefined, model: undefined })}
                        className={`px-4 py-2 text-sm font-semibold rounded-lg transition ${
                          active
                            ? 'bg-blue-500 text-white shadow-sm'
                            : disabled
                            ? 'text-muted opacity-40 cursor-not-allowed'
                            : 'text-secondary hover:text-primary hover:bg-elevated/70'
                        }`}
                      >
                        {SERVICE_TAB_LABELS[tab]}
                      </button>
                    );
                  })}
                </div>
              ) : (
                <span className="inline-flex rounded-full border border-default/70 bg-surface/70 px-3 py-1.5 text-sm font-medium text-primary">
                  {currentServiceViewLabel}
                </span>
              )}
              <span className="text-xs text-muted">当前同步分组：{currentServiceGroup}</span>
            </div>
          </section>

          <section className="mb-4 rounded-2xl border border-default/70 bg-surface/55 px-4 py-4">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div>
                <h2 className="text-lg font-semibold text-primary">同步通道</h2>
                <p className="mt-1 text-sm text-secondary">
                  下列通道全部来自 `new-api` 同步快照。点击后切换当前详情页的通道视图。
                </p>
              </div>
              <div className="text-xs text-muted">
                当前类型：{currentSnapshot?.channelTypeLabel || currentSourceMeta?.label || '未知'}
              </div>
            </div>

            <div className="grid gap-3 lg:grid-cols-2">
              {sourceFilteredSnapshots.map((snapshot) => {
                const active = snapshot.channel === selectedChannel;
                const modelsCount = splitModels(snapshot.model).length;
                return (
                  <button
                    key={snapshot.channel}
                    type="button"
                    onClick={() => updateParam(setSearchParams, searchParams, { channel: snapshot.channel, model: undefined })}
                    className={`rounded-xl border px-4 py-4 text-left transition ${
                      active
                        ? 'border-accent bg-accent/10 shadow-sm'
                        : 'border-default/70 bg-surface/65 hover:bg-elevated/45'
                    }`}
                  >
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div>
                        <div className="font-medium text-primary">{extractAuditChannelName(snapshot.channel)}</div>
                        <div className="mt-1 text-xs text-secondary">{snapshot.channel}</div>
                      </div>
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="inline-flex items-center rounded-full border border-default/70 bg-surface/70 px-2.5 py-1 text-xs text-secondary">
                          {snapshot.channelTypeLabel || inferSourceKey(snapshot)}
                        </span>
                        <SnapshotStatusBadge snapshot={snapshot} compact />
                      </div>
                    </div>
                    <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted">
                      <span>模型 {modelsCount}</span>
                      <span>分组 {snapshot.service || '--'}</span>
                    </div>
                  </button>
                );
              })}
            </div>
          </section>

          <section className="mb-4 grid gap-4 lg:grid-cols-4 md:grid-cols-2">
            <SummaryCard
              label="当前通道"
              value={currentSnapshot ? extractAuditChannelName(currentSnapshot.channel) : '--'}
              hint={currentSnapshot?.channel || '未选定通道'}
            />
            <SummaryCard
              label="当前状态"
              value={currentSnapshot ? <SnapshotStatusBadge snapshot={currentSnapshot} compact /> : '--'}
              hint={currentSnapshot?.channelTypeLabel || '以同步快照为准'}
            />
            <SummaryCard
              label="模型数量"
              value={String(modelRows.length)}
              hint={selectedModel === 'all' ? '当前通道全部模型' : selectedModel}
            />
            <SummaryCard
              label="同步分组"
              value={currentServiceGroup}
              hint={`当前服务视图：${currentServiceViewLabel}`}
            />
          </section>

          <section className="mb-4 rounded-2xl border border-default/70 bg-surface/55 px-4 py-3 text-sm text-secondary">
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
              <span>最近有效样本 {diagnosticSummary.usable}</span>
              <span>401失败 {diagnosticSummary.failedAuth}</span>
              <span>请求失败 {diagnosticSummary.failedRequest}</span>
              <span>其他 {diagnosticSummary.pending}</span>
            </div>
          </section>

          <section className="mb-4 rounded-2xl border border-default/70 bg-surface/55 px-4 py-4">
            <div className="mb-3 text-sm font-semibold text-primary">筛选当前详情</div>
            <div className="grid gap-4 md:grid-cols-3">
              <FilterField
                label="通道类型"
                value={selectedSource}
                onChange={(value) => updateParam(setSearchParams, searchParams, { source: value === 'all' ? undefined : value, channel: undefined, model: undefined })}
                options={sourceOptions.map((option) => ({
                  value: option,
                  label: option === 'all' ? '全部类型' : SOURCE_META[option as Exclude<SourceKey, 'all'>].label,
                }))}
              />
              <FilterField
                label="通道"
                value={selectedChannel}
                onChange={(value) => updateParam(setSearchParams, searchParams, { channel: value, model: undefined })}
                options={channelOptions}
              />
              <FilterField
                label="模型"
                value={selectedModel}
                onChange={(value) => updateParam(setSearchParams, searchParams, { model: value === 'all' ? undefined : value })}
                options={[{ value: 'all', label: '全部模型' }, ...modelOptions.map((model) => ({ value: model, label: model }))]}
              />
            </div>
          </section>

          {showProbeWarning && (
            <section className="mb-6 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-4 text-sm">
              <div className="font-semibold text-amber-200">当前通道尚无有效检测样本</div>
              <p className="mt-1 text-amber-100 leading-relaxed">
                {syncStatus?.probe_runtime.warning}
              </p>
              <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-amber-200/90">
                <span>同步目标 {syncStatus?.targets.enabled ?? 0}/{syncStatus?.targets.total ?? 0}</span>
                <span>渠道快照 {syncStatus?.channels?.channel_count ?? 0}</span>
                <span>凭证模式 {syncStatus?.probe_runtime.probe_credential_mode ?? 'missing'}</span>
              </div>
            </section>
          )}

          <section className="mb-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 className="text-lg font-semibold text-primary">模型状态</h2>
                <p className="mt-1 text-sm text-secondary">
                  当前表格按模型展开，优先展示模型是否启用、最近检测状态和可用率趋势。
                </p>
              </div>
              {currentSourceMeta ? (
                <div className="inline-flex items-center gap-2 rounded-full border border-default/70 bg-surface/70 px-3 py-1 text-xs text-secondary">
                  {currentSourceMeta.icon}
                  <span>{currentSnapshot?.channelTypeLabel || currentSourceMeta.label}</span>
                </div>
              ) : null}
            </div>
          </section>

          <main className="overflow-x-auto rounded-2xl border border-default/70 bg-surface/55 shadow-xl backdrop-blur-sm">
            {(auditLoading || monitorLoading || latestDiagnosticsLoading) && modelRows.length === 0 ? (
              <div className="px-6 py-16 text-center text-muted">正在加载模型级详情…</div>
            ) : auditError || monitorError || latestDiagnosticsError ? (
              <div className="px-6 py-16 text-center text-danger">
                {auditError || monitorError || latestDiagnosticsError}
              </div>
            ) : !currentSnapshot ? (
              <div className="px-6 py-16 text-center text-muted">当前筛选下没有可展示的通道。</div>
            ) : (
              <table className="w-full min-w-[1280px] text-left">
                <thead>
                  <tr className="border-b border-default/60 text-[13px] text-secondary">
                    <th className="px-4 py-4 font-medium">模型</th>
                    <th className="px-4 py-4 font-medium">当前状态</th>
                    <th className="px-4 py-4 font-medium">最近检测状态</th>
                    <th className="px-4 py-4 font-medium">最终质量分</th>
                    <th className="px-4 py-4 font-medium">可用率 30D</th>
                    <th className="px-4 py-4 font-medium">趋势</th>
                    <th className="px-4 py-4 font-medium">结果详情</th>
                  </tr>
                </thead>
                <tbody>
                  {modelRows.map((row, index) => (
                    <tr key={row.id} className={`border-b border-default/40 ${index === 0 ? 'bg-white/[0.03]' : 'hover:bg-elevated/35'} transition-colors`}>
                      <td className="px-4 py-4">
                        <div className="space-y-1">
                          <div className="font-mono text-primary">{row.modelName}</div>
                          <div className="flex flex-wrap items-center gap-2 text-xs text-secondary">
                            {currentSourceMeta?.icon}
                            <span>{currentSnapshot?.channelTypeLabel || currentSourceMeta?.label || '未知'}</span>
                            <span className="text-muted">/</span>
                            <span>{currentSnapshot ? extractAuditChannelName(currentSnapshot.channel) : '--'}</span>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-4">
                        {currentSnapshot ? <SnapshotStatusBadge snapshot={currentSnapshot} /> : <CurrentStatusBadge enabled={row.enabled} />}
                      </td>
                      <td className="px-4 py-4">
                        <div className="space-y-1">
                          {row.latestAttemptStatus ? (
                            <div className="flex flex-wrap items-center gap-2 text-xs">
                              <LatestAttemptStatusBadge status={row.latestAttemptStatus} />
                              {row.latestAttemptCreatedAt ? (
                                <span className="text-muted">{formatDateTime(row.latestAttemptCreatedAt)}</span>
                              ) : null}
                            </div>
                          ) : (
                            <span className="text-muted text-sm">{showProbeWarning ? '等待有效样本' : '暂无检测记录'}</span>
                          )}
                          {row.latestAttemptReason ? (
                            <div className="max-w-[18rem] text-xs leading-relaxed text-amber-300">
                              {row.latestAttemptReason}
                            </div>
                          ) : null}
                        </div>
                      </td>
                      <td className="px-4 py-4">
                        <div className="flex flex-wrap items-center gap-2">
                          <ScoreBadge score={row.finalScore} enabled={row.enabled} />
                          {row.fingerprintScore != null ? (
                            <span className="text-xs text-muted">指纹 {Math.round(row.fingerprintScore)}</span>
                          ) : null}
                        </div>
                      </td>
                      <td className="px-4 py-4">
                        <AvailabilityBadge value={row.uptime} enabled={row.enabled} />
                      </td>
                      <td className="px-4 py-4">
                        <TrendSparkline trend={row.trend} enabled={row.enabled} />
                      </td>
                      <td className="px-4 py-4">
                        <div className="flex flex-col items-start gap-2">
                          {row.compareUrl ? (
                            <a
                              href={row.compareUrl}
                              target="_blank"
                              rel="noreferrer"
                              className="inline-flex items-center rounded-md bg-blue-500/15 px-2 py-1 text-xs font-semibold text-blue-300 hover:bg-blue-500/20"
                            >
                              {row.latestAttemptStatus && row.latestAttemptStatus !== 'done'
                                ? '失败详情'
                                : (row.latestMethodologyVersion || '查看结果')}
                            </a>
                          ) : (
                            <span className="text-muted text-sm">
                              {showProbeWarning ? '等待有效样本' : '暂无'}
                            </span>
                          )}
                          <div className="text-xs text-muted">
                            测试数 {row.testsCount ?? '--'}
                            {row.avgLatencyMs != null ? ` / 均延迟 ${formatLatency(row.avgLatencyMs)}` : ''}
                            {row.p95LatencyMs != null ? ` / p95 ${formatLatencyCompact(row.p95LatencyMs)}` : ''}
                          </div>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </main>

          {!rpdiagEnabled && (
            <div className="mt-4 rounded-xl border border-amber-500/20 bg-amber-500/8 px-4 py-3 text-sm text-amber-200">
              当前部署未接入 rpdiag 质量分；模型列表仍来自 new-api，同步成功，但质量列将以占位态展示。
            </div>
          )}
          {rpdiagEnabled && rpdiagLoaded && Object.keys(rpdiagScores).length === 0 && (
            <div className="mt-4 rounded-xl border border-slate-500/20 bg-slate-500/8 px-4 py-3 text-sm text-slate-300">
              当前 rpdiag 质量索引为空，模型已展开显示，但质量分与趋势暂无可用数据。
            </div>
          )}
        </div>
      </div>
    </>
  );
}

function FilterField({
  label,
  value,
  options,
  onChange,
  disabled = false,
}: {
  label: string;
  value: string;
  options: Array<{ value: string; label: string }>;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return (
    <label className="block">
      <div className="mb-2 text-sm text-secondary">{label}</div>
      <select
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        className="w-full rounded-xl border border-default/70 bg-surface/75 px-4 py-3 text-sm text-primary outline-none transition focus:border-accent disabled:cursor-not-allowed disabled:opacity-70"
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

function SummaryCard({ label, value, hint }: { label: string; value: React.ReactNode; hint?: string }) {
  return (
    <div className="rounded-xl border border-default/70 bg-surface/70 px-4 py-4">
      <div className="text-sm text-secondary">{label}</div>
      <div className="mt-2 text-xl font-semibold text-primary break-all">{value}</div>
      {hint ? <div className="mt-1 text-xs leading-relaxed text-muted">{hint}</div> : null}
    </div>
  );
}

function ScoreBadge({
  score,
  enabled,
  subtle = false,
}: {
  score: number | null;
  enabled: boolean;
  subtle?: boolean;
}) {
  if (score == null) {
    if (!enabled) {
      return <span className="inline-flex rounded-md bg-slate-500/15 px-2 py-1 text-xs font-semibold text-slate-300">不可测</span>;
    }
    return <span className="inline-flex rounded-md bg-slate-500/15 px-2 py-1 text-xs font-semibold text-slate-300">暂无</span>;
  }

  let className = 'bg-emerald-500/15 text-emerald-300';
  if (score < 50) className = 'bg-rose-500/15 text-rose-300';
  else if (score < 70) className = 'bg-amber-500/15 text-amber-300';
  else if (score < 85) className = 'bg-lime-500/15 text-lime-300';

  return (
    <span
      className={`inline-flex min-w-[3.25rem] justify-center rounded-full px-3 py-1 text-sm font-bold ${className} ${subtle ? 'opacity-90' : ''}`}
    >
      {Math.round(score)}
    </span>
  );
}

function CurrentStatusBadge({ enabled }: { enabled: boolean }) {
  if (enabled) {
    return <span className="inline-flex rounded-full bg-emerald-500/15 px-3 py-1 text-sm font-semibold text-emerald-300">启用</span>;
  }
  return <span className="inline-flex rounded-full bg-slate-500/15 px-3 py-1 text-sm font-semibold text-slate-300">停用</span>;
}

function LatestAttemptStatusBadge({ status }: { status: string }) {
  const normalized = status.trim().toLowerCase();
  let label = status || '未知';
  let className = 'bg-slate-500/15 text-slate-300';

  switch (normalized) {
    case 'failed_auth':
      label = '认证失败';
      className = 'bg-rose-500/15 text-rose-300';
      break;
    case 'failed_request':
      label = '请求失败';
      className = 'bg-amber-500/15 text-amber-300';
      break;
    case 'usable':
    case 'done':
      label = '已完成';
      className = 'bg-emerald-500/15 text-emerald-300';
      break;
    case 'not_done':
      label = '进行中';
      className = 'bg-sky-500/15 text-sky-300';
      break;
    default:
      break;
  }

  return <span className={`inline-flex rounded-md px-2 py-0.5 font-semibold ${className}`}>{label}</span>;
}

function SnapshotStatusBadge({ snapshot, compact = false }: { snapshot: AuditChannelSnapshot; compact?: boolean }) {
  const rawStatus = typeof snapshot.raw?.Status === 'number' ? snapshot.raw.Status : null;
  const className = compact
    ? 'inline-flex rounded-full px-2.5 py-1 text-xs font-semibold'
    : 'inline-flex rounded-full px-3 py-1 text-sm font-semibold';
  if (snapshot.enabled) {
    return <span className={`${className} bg-emerald-500/15 text-emerald-300`}>已启用</span>;
  }
  if (rawStatus != null) {
    return (
      <span className={`${className} bg-slate-500/15 text-slate-300`}>
        {`已禁用(S${rawStatus})`}
      </span>
    );
  }
  return <span className={`${className} bg-slate-500/15 text-slate-300`}>已停用</span>;
}

function AvailabilityBadge({ value, enabled }: { value: number | null; enabled: boolean }) {
  if (!enabled) {
    return <span className="inline-flex rounded-full bg-slate-500/15 px-3 py-1 text-sm font-semibold text-slate-300">不可测</span>;
  }
  if (value == null || value < 0) {
    return <span className="inline-flex rounded-full bg-slate-500/15 px-3 py-1 text-sm font-semibold text-slate-300">--</span>;
  }
  return (
    <span className="inline-flex rounded-full bg-emerald-500/15 px-3 py-1 text-sm font-semibold text-emerald-300">
      {value.toFixed(value >= 100 ? 0 : 1)}%
    </span>
  );
}

function TrendSparkline({ trend, enabled }: { trend?: RpdiagScoreTrend | null; enabled: boolean }) {
  const points = buildTrendPoints(trend);
  if (points.length === 0) {
    return (
      <div className="flex items-center gap-1">
        {[0, 1, 2, 3, 4].map((index) => (
          <span
            key={index}
            className={`inline-block h-1.5 w-1.5 rounded-full ${enabled ? 'bg-slate-500/40' : 'bg-slate-600/40'}`}
          />
        ))}
      </div>
    );
  }

  const width = 54;
  const height = 16;
  const xStep = points.length === 1 ? 0 : width / (points.length - 1);
  const coords = points.map((point, index) => {
    const normalized = point == null ? 0 : Math.max(0, Math.min(100, point));
    const x = index * xStep;
    const y = height - (normalized / 100) * (height - 2) - 1;
    return { x, y, value: point };
  });
  const line = coords
    .filter((point) => point.value != null)
    .map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x.toFixed(1)} ${point.y.toFixed(1)}`)
    .join(' ');

  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} className="overflow-visible">
      {line && <path d={line} fill="none" stroke="hsl(92 80% 58%)" strokeWidth="1.5" strokeLinecap="round" />}
      {coords.map((point, index) => (
        <circle
          key={index}
          cx={point.x}
          cy={point.y}
          r="1.8"
          fill={point.value == null ? 'hsl(215 16% 55%)' : point.value >= 70 ? 'hsl(92 80% 58%)' : point.value >= 50 ? 'hsl(35 90% 58%)' : 'hsl(350 85% 60%)'}
        />
      ))}
    </svg>
  );
}

function buildTrendPoints(trend?: RpdiagScoreTrend | null): Array<number | null> {
  if (!trend) return [];
  if (Array.isArray(trend.recent_attempts) && trend.recent_attempts.length > 0) {
    return trend.recent_attempts.slice(-5);
  }
  if (Array.isArray(trend.recent_scores) && trend.recent_scores.length > 0) {
    return trend.recent_scores.slice(-5);
  }
  return [trend.avg_30d ?? null, trend.avg_7d ?? null, trend.latest ?? null].filter((item) => item !== undefined);
}

function splitModels(raw: string): string[] {
  return raw
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}

function normalizeModelKey(value?: string | null): string {
  return (value || '').trim().toLowerCase();
}

function pickTestsCount(trend?: RpdiagScoreTrend | null): number | null {
  if (!trend) return null;
  if (typeof trend.n_30d === 'number' && trend.n_30d > 0) return trend.n_30d;
  if (typeof trend.n_7d === 'number' && trend.n_7d > 0) return trend.n_7d;
  return null;
}

function computeAverageLatency(history?: ProcessedMonitorData['history']): number | null {
  if (!history || history.length === 0) return null;
  const values = history.map((point) => point.latency).filter((value) => value > 0);
  if (values.length === 0) return null;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function computeP95Latency(history?: ProcessedMonitorData['history']): number | null {
  if (!history || history.length === 0) return null;
  const values = history.map((point) => point.latency).filter((value) => value > 0).sort((a, b) => a - b);
  if (values.length === 0) return null;
  const index = Math.min(values.length - 1, Math.floor(values.length * 0.95));
  return values[index];
}

function formatLatency(value: number | null): string {
  if (value == null || value <= 0) return '--';
  return `${(value / 1000).toFixed(value >= 10_000 ? 1 : 2)}s`;
}

function formatLatencyCompact(value: number): string {
  return `${(value / 1000).toFixed(1)}s`;
}

function formatDateTime(unixSeconds: number): string {
  if (!unixSeconds || unixSeconds <= 0) return '--';
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(unixSeconds * 1000));
}

function inferSourceKey(snapshot: AuditChannelSnapshot): SourceKey {
  const typed = (snapshot.channelType || '').toLowerCase();
  if (typed === 'official') return 'official';
  if (typed === 'reverse') return 'reverse';
  if (typed === 'mixed') return 'mixed';
  if (typed === 'unknown') return 'unknown';

  const type = parseChannelType(snapshot.channel);
  if (type === 'official') return 'official';
  if (type === 'reverse') return 'reverse';
  if (type === 'mixed') return 'mixed';

  const rawText = JSON.stringify(snapshot.raw || {}).toLowerCase();
  if (rawText.includes('recommend') || rawText.includes('董推')) return 'recommended';
  if (rawText.includes('user') || rawText.includes('submit')) return 'user';
  return 'unknown';
}

function updateParam(
  setSearchParams: ReturnType<typeof useSearchParams>[1],
  current: URLSearchParams,
  values: Record<string, string | undefined>,
) {
  const next = new URLSearchParams(current);
  Object.entries(values).forEach(([key, value]) => {
    if (!value) next.delete(key);
    else next.set(key, value);
  });
  setSearchParams(next);
}
