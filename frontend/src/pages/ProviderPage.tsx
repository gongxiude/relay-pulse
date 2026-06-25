import { useEffect, useMemo } from 'react';
import { useLocation, useParams, useSearchParams } from 'react-router-dom';
import { Helmet } from 'react-helmet-async';
import { CircleHelp, Flame, Sparkles } from 'lucide-react';

import { Header } from '../components/Header';
import { Footer } from '../components/Footer';
import { ChannelTypeIcon, parseChannelType } from '../components/ChannelTypeIcon';
import { useAuditChannels } from '../hooks/useAuditChannels';
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
  rankScore: number;
  providerName: string;
  channelLabel: string;
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
  sourceKey: SourceKey;
}

const SERVICE_TAB_LABELS: Record<ServiceTab, string> = {
  cc: 'Claude Code',
  cx: 'Codex',
};

const SCORE_BANDS = [
  { label: '≥85', desc: '与官方近 3 次基线高度接近', color: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30' },
  { label: '70-84', desc: '多数维度接近基线', color: 'bg-lime-500/15 text-lime-300 border-lime-500/30' },
  { label: '50-69', desc: '与基线存在可见偏离', color: 'bg-amber-500/15 text-amber-300 border-amber-500/30' },
  { label: '<50', desc: '与基线偏离较明显', color: 'bg-rose-500/15 text-rose-300 border-rose-500/30' },
];

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

  const normalizedProvider = canonicalize(provider);
  const { channels: auditChannels, loading: auditLoading, error: auditError } = useAuditChannels();
  const { scores: rpdiagScores, loaded: rpdiagLoaded } = useRpdiagScores();
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

  const modelOptions = useMemo(() => {
    if (!currentSnapshot) return [];
    return splitModels(currentSnapshot.model);
  }, [currentSnapshot]);

  const selectedModel = useMemo(() => {
    const value = searchParams.get('model') || 'all';
    return value === 'all' || modelOptions.includes(value) ? value : 'all';
  }, [searchParams, modelOptions]);

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
        return {
          id: `${currentSnapshot.newapi_channel_id}-${modelName}`,
          rankScore: rpModel?.score ?? -1,
          providerName: currentSnapshot.provider,
          channelLabel: extractAuditChannelName(currentSnapshot.channel),
          modelName,
          finalScore: rpModel?.score ?? null,
          fingerprintScore: rpModel?.score ?? currentRpdiag?.max_score ?? null,
          trend: rpModel?.trend,
          testsCount: pickTestsCount(rpModel?.trend),
          uptime: matchedMonitor?.uptime ?? null,
          avgLatencyMs: computeAverageLatency(matchedMonitor?.history),
          p95LatencyMs: computeP95Latency(matchedMonitor?.history),
          ttftMs: null,
          enabled: currentSnapshot.enabled,
          sourceKey: inferSourceKey(currentSnapshot),
        } satisfies ModelDetailRow;
      })
      .sort((a, b) => {
        if (b.rankScore !== a.rankScore) return b.rankScore - a.rankScore;
        return a.modelName.localeCompare(b.modelName);
      });

    return filtered.map((row, index) => ({
      ...row,
      id: `${row.id}-${index}`,
    }));
  }, [currentSnapshot, currentRpdiag, matchedMonitor, selectedModel]);

  const headerStats = useMemo(() => {
    const total = modelRows.length;
    const healthy = modelRows.filter((row) => row.enabled && (row.uptime ?? 0) > 0).length;
    return {
      total,
      healthy,
      issues: Math.max(0, total - healthy),
    };
  }, [modelRows]);

  const pageTitle = providerDisplayName ? `${providerDisplayName} - 服务商质量排名` : '服务商质量排名';

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

      <div className="min-h-screen bg-page text-primary font-sans selection-accent overflow-x-hidden">
        <div className="fixed top-0 left-0 w-full h-full overflow-hidden pointer-events-none z-0">
          <div className="absolute top-[-12%] right-[-8%] w-[560px] h-[560px] bg-accent/8 rounded-full blur-[120px]" />
          <div className="absolute bottom-[-18%] left-[-8%] w-[520px] h-[520px] bg-cyan-500/8 rounded-full blur-[120px]" />
        </div>

        <div className="relative z-10 max-w-7xl mx-auto px-4 py-6 sm:px-6 lg:px-8">
          <Header stats={headerStats} />

          <section className="mb-6">
            <h1 className="text-3xl font-bold tracking-tight text-primary">服务商质量排名</h1>
            <p className="mt-3 text-secondary text-base leading-relaxed">
              基于协议指纹与官方近 3 次基线的加权相似度评分（0-100），分数越高代表与官方行为越接近。
            </p>
            <div className="mt-4 rounded-xl border border-default/70 bg-surface/70 px-4 py-3 text-sm text-muted">
              仅供参考的统计观察；权重见方法论，不构成定性结论。
            </div>
          </section>

          <section className="mb-4">
            <div className="flex items-center gap-3">
              <span className="text-sm text-secondary min-w-[2rem]">服务</span>
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
            </div>
          </section>

          <section className="mb-4 grid gap-4 md:grid-cols-4">
            <FilterField
              label="来源"
              value={selectedSource}
              onChange={(value) => updateParam(setSearchParams, searchParams, { source: value === 'all' ? undefined : value, channel: undefined, model: undefined })}
              options={sourceOptions.map((option) => ({
                value: option,
                label: option === 'all' ? '全部来源' : SOURCE_META[option as Exclude<SourceKey, 'all'>].label,
              }))}
            />
            <FilterField
              label="服务商"
              value={providerDisplayName}
              disabled
              options={[{ value: providerDisplayName, label: providerDisplayName }]}
              onChange={() => {}}
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
          </section>

          <section className="mb-3 rounded-xl border border-default/70 bg-surface/70 px-4 py-3 text-sm">
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-secondary">
              <span className="text-muted">通道符号:</span>
              <LegendChip icon={<Sparkles size={14} className="text-cyan-300" />} label="董推" />
              <LegendChip icon={<Flame size={14} className="text-orange-300" />} label="rpdiag 直连基准" />
              <LegendChip icon={<ChannelTypeIcon channel="O-demo" />} label="服务商自报官方通道 (O-)" />
              <LegendChip icon={<ChannelTypeIcon channel="R-demo" />} label="逆向 (R-)" />
              <LegendChip icon={<ChannelTypeIcon channel="M-demo" />} label="混合 (M-)" />
              <LegendChip icon={<ChannelTypeIcon channel="X-demo" />} label="未知" />
              <LegendChip icon={<CircleHelp size={14} className="text-slate-300" />} label="用户提交 (U-)" />
              {!currentSnapshot?.enabled && <span className="inline-flex items-center gap-1 rounded-md bg-slate-500/15 px-2 py-0.5 text-slate-300">不可测</span>}
              <span className="text-muted">最近一次未取回可评分响应（排名按 0 计）</span>
            </div>
          </section>

          <section className="mb-6 rounded-xl border border-default/70 bg-surface/70 px-4 py-3 text-sm">
            <div className="flex flex-wrap items-center gap-3">
              <span className="text-muted">分数含义:</span>
              {SCORE_BANDS.map((band) => (
                <div key={band.label} className="flex items-center gap-2">
                  <span className={`inline-flex items-center rounded-full border px-2.5 py-0.5 font-semibold ${band.color}`}>
                    {band.label}
                  </span>
                  <span className="text-secondary">{band.desc}</span>
                </div>
              ))}
            </div>
            <p className="mt-2 text-muted">
              分数为多维加权相似度（含可用率 / 延迟等展示层调整），非通道争纷或质量定性结论。
            </p>
          </section>

          <main className="overflow-x-auto rounded-2xl border border-default/70 bg-surface/55 shadow-xl backdrop-blur-sm">
            {(auditLoading || monitorLoading) && modelRows.length === 0 ? (
              <div className="px-6 py-16 text-center text-muted">正在加载模型级详情…</div>
            ) : auditError || monitorError ? (
              <div className="px-6 py-16 text-center text-danger">
                {auditError || monitorError}
              </div>
            ) : !currentSnapshot ? (
              <div className="px-6 py-16 text-center text-muted">当前筛选下没有可展示的通道。</div>
            ) : (
              <table className="w-full min-w-[1180px] text-left">
                <thead>
                  <tr className="border-b border-default/60 text-[13px] text-secondary">
                    <th className="px-4 py-4 font-medium">#</th>
                    <th className="px-4 py-4 font-medium">服务商</th>
                    <th className="px-4 py-4 font-medium">通道</th>
                    <th className="px-4 py-4 font-medium">模型</th>
                    <th className="px-4 py-4 font-medium">最终质量分</th>
                    <th className="px-4 py-4 font-medium">机器指纹分</th>
                    <th className="px-4 py-4 font-medium">趋势</th>
                    <th className="px-4 py-4 font-medium">测试数</th>
                    <th className="px-4 py-4 font-medium">可用率 30D</th>
                    <th className="px-4 py-4 font-medium">首 TOKEN 30D</th>
                    <th className="px-4 py-4 font-medium">平均延迟 30D</th>
                  </tr>
                </thead>
                <tbody>
                  {modelRows.map((row, index) => (
                    <tr key={row.id} className={`border-b border-default/40 ${index === 0 ? 'bg-white/[0.03]' : 'hover:bg-elevated/35'} transition-colors`}>
                      <td className="px-4 py-4 font-mono text-secondary">{index + 1}</td>
                      <td className="px-4 py-4">
                        <div className="font-semibold text-primary">{row.providerName}</div>
                      </td>
                      <td className="px-4 py-4">
                        <div className="flex items-center gap-2 text-secondary">
                          {row.sourceKey === 'all'
                            ? <ChannelTypeIcon channel={currentSnapshot.channel} />
                            : SOURCE_META[row.sourceKey].icon}
                          <span className="font-medium text-blue-300">{row.channelLabel}</span>
                        </div>
                      </td>
                      <td className="px-4 py-4 font-mono text-primary">{row.modelName}</td>
                      <td className="px-4 py-4">
                        <ScoreBadge score={row.finalScore} enabled={row.enabled} />
                      </td>
                      <td className="px-4 py-4">
                        <ScoreBadge score={row.fingerprintScore} enabled={true} subtle />
                      </td>
                      <td className="px-4 py-4">
                        <TrendSparkline trend={row.trend} enabled={row.enabled} />
                      </td>
                      <td className="px-4 py-4 font-medium text-primary">{row.testsCount ?? '--'}</td>
                      <td className="px-4 py-4">
                        <AvailabilityBadge value={row.uptime} enabled={row.enabled} />
                      </td>
                      <td className="px-4 py-4 font-medium text-primary">
                        {formatLatency(row.ttftMs)}
                      </td>
                      <td className="px-4 py-4">
                        <div className="flex items-center gap-2">
                          <span className="font-medium text-primary">{formatLatency(row.avgLatencyMs)}</span>
                          {row.p95LatencyMs != null && (
                            <span className="inline-flex items-center rounded-md bg-orange-500/15 px-2 py-0.5 text-xs font-semibold text-orange-300">
                              p95 {formatLatencyCompact(row.p95LatencyMs)}
                            </span>
                          )}
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

          <Footer />
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

function LegendChip({ icon, label }: { icon: React.ReactNode; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-secondary">
      {icon}
      <span>{label}</span>
    </span>
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

function inferSourceKey(snapshot: AuditChannelSnapshot): SourceKey {
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
