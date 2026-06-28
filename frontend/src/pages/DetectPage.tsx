import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Helmet } from 'react-helmet-async';
import { useSearchParams } from 'react-router-dom';
import {
  Activity,
  ShieldQuestion,
  Fingerprint,
  ListChecks,
  HelpCircle,
  ExternalLink,
} from 'lucide-react';

import { Header } from '../components/Header';
import { LANGUAGE_PATH_MAP, type SupportedLanguage } from '../i18n';
import { useAuditMethodology } from '../hooks/useAuditMethodology';
import { useAuditDiagnosticLatest } from '../hooks/useAuditDiagnosticLatest';
import { useRpdiagScores, lookupRpdiagScore } from '../hooks/useRpdiagScores';
import { useMonitorData } from '../hooks/useMonitorData';
import type { AuditDiagnosticLatestItem, AuditMethodologyDimension } from '../types/audit';

/** 质量分色带：与 StatusTable.qualityScoreColor 同一套 hue 阶梯（红→翠绿）。
 *  StatusTable 内为私有函数，这里独立一份避免改动那个组件；阈值变更需两处同步。 */
function scoreColor(score: number): string {
  const stops: Array<[number, number, number, number]> = [
    [0, 0, 78, 50],
    [60, 40, 82, 50],
    [80, 75, 72, 48],
    [90, 105, 70, 46],
    [100, 140, 78, 44],
  ];
  const c = Math.max(0, Math.min(100, score));
  for (let i = 1; i < stops.length; i++) {
    if (c <= stops[i][0]) {
      const [s0, h0, sat0, l0] = stops[i - 1];
      const [s1, h1, sat1, l1] = stops[i];
      const t = s1 === s0 ? 0 : (c - s0) / (s1 - s0);
      return `hsl(${h0 + t * (h1 - h0)} ${sat0 + t * (sat1 - sat0)}% ${l0 + t * (l1 - l0)}%)`;
    }
  }
  return 'hsl(140 78% 44%)';
}

interface RankRow {
  id: string;
  providerName: string;
  providerSlug: string;
  channelLabel: string;
  serviceName: string;
  score: number;
  uptime: number;
  evidenceUrl?: string;
}

/** 实时质量排行：复用 useMonitorData（已按三元组 join rpdiag 质量分并带展示名/slug）
 *  + useRpdiagScores（取每行的 channel_url 作证据深链）。
 *
 *  关键的「正派」约束：纯按质量分降序，isInitialSort=false 关掉赞助置顶——质量排行不能
 *  让赞助商浮到真实分之上。展示名来自 relaypulse 自己的通道清单，不是 rpdiag map 的小写 key。 */
function QualityRanking() {
  const { t, i18n } = useTranslation();
  const langPrefix = LANGUAGE_PATH_MAP[i18n.language as SupportedLanguage];
  const providerHref = (slug: string) => (langPrefix ? `/${langPrefix}/p/${slug}` : `/p/${slug}`);
  const { scores, loaded } = useRpdiagScores();
  const { data, loading } = useMonitorData({
    timeRange: '90m',
    board: 'all',
    filterService: [],
    filterProvider: [],
    filterChannel: [],
    filterCategory: [],
    sortConfig: { key: 'qualityScore', direction: 'desc' },
    isInitialSort: false,
    rpdiagScores: scores,
    rpdiagScoresLoaded: loaded,
  });

  const rows = useMemo<RankRow[]>(() => {
    return data
      .filter((d) => typeof d.qualityScore === 'number')
      .map((d) => {
        const evidence = lookupRpdiagScore(
          scores,
          d.providerId,
          d.serviceType,
          d.channelName || d.channel,
        );
        return {
          id: d.id,
          providerName: d.providerName,
          providerSlug: d.providerSlug,
          channelLabel: d.channelName || d.channel || '',
          serviceName: d.serviceName,
          score: d.qualityScore as number,
          uptime: d.uptime,
          evidenceUrl: evidence?.channel_url,
        };
      })
      .sort((a, b) => b.score - a.score);
  }, [data, scores]);

  if (loading && rows.length === 0) {
    return <p className="text-secondary text-sm py-8 text-center">{t('detect.ranking.loading')}</p>;
  }
  if (rows.length === 0) {
    return <p className="text-secondary text-sm py-8 text-center">{t('detect.ranking.empty')}</p>;
  }

  return (
    <div className="overflow-x-auto rounded-2xl border border-default bg-surface">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-muted border-b border-default/60">
            <th className="px-3 py-3 font-medium w-10">#</th>
            <th className="px-3 py-3 font-medium">{t('detect.ranking.col.provider')}</th>
            <th className="px-3 py-3 font-medium">{t('detect.ranking.col.channel')}</th>
            <th className="px-3 py-3 font-medium text-right">{t('detect.ranking.col.score')}</th>
            <th className="px-3 py-3 font-medium text-right whitespace-nowrap">{t('detect.ranking.col.uptime')}</th>
            <th className="px-3 py-3 font-medium text-right">{t('detect.ranking.col.evidence')}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={r.id} className="border-b border-default/30 hover:bg-elevated/40 transition-colors">
              <td className="px-3 py-2.5 text-muted font-mono">{i + 1}</td>
              <td className="px-3 py-2.5">
                {r.providerSlug ? (
                  <a
                    href={providerHref(r.providerSlug)}
                    className="text-primary font-medium hover:text-accent transition-colors"
                  >
                    {r.providerName}
                  </a>
                ) : (
                  <span className="text-primary font-medium">{r.providerName}</span>
                )}
                <span className="text-muted ml-2 text-xs">{r.serviceName}</span>
              </td>
              <td className="px-3 py-2.5 text-secondary">{r.channelLabel}</td>
              <td className="px-3 py-2.5 text-right">
                <span
                  className="inline-block min-w-[2.75rem] rounded-md px-2 py-0.5 font-mono font-bold text-[hsl(0_0%_100%)]"
                  style={{ backgroundColor: scoreColor(r.score) }}
                >
                  {r.score.toFixed(0)}
                </span>
              </td>
              <td className="px-3 py-2.5 text-right font-mono text-secondary">
                {r.uptime >= 0 ? `${r.uptime.toFixed(0)}%` : '—'}
              </td>
              <td className="px-3 py-2.5 text-right">
                {r.evidenceUrl ? (
                  <a
                    href={r.evidenceUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 text-accent hover:underline whitespace-nowrap"
                  >
                    {t('detect.ranking.col.evidenceLink')}
                    <ExternalLink size={12} />
                  </a>
                ) : (
                  <span className="text-muted">—</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Section({
  icon: Icon,
  title,
  children,
}: {
  icon: React.ElementType;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section className="mb-14">
      <h2 className="flex items-center gap-2.5 text-2xl font-bold text-primary mb-5">
        <span className="flex items-center justify-center w-9 h-9 rounded-lg bg-accent/10 text-accent flex-shrink-0">
          <Icon size={20} />
        </span>
        {title}
      </h2>
      {children}
    </section>
  );
}

function MetricCard({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <div className="rounded-xl border border-default bg-surface p-4">
      <div className="text-sm text-muted mb-1">{label}</div>
      <div className="text-xl font-semibold text-primary break-all">{value}</div>
      <div className="text-xs text-secondary mt-1 break-all">{hint}</div>
    </div>
  );
}

function DimensionStatusBadge({ dimension }: { dimension: AuditMethodologyDimension }) {
  if (dimension.active) {
    return <span className="inline-flex rounded-full bg-emerald-500/15 px-2.5 py-1 text-xs font-semibold text-emerald-300">已启用</span>;
  }
  if (dimension.implemented) {
    return <span className="inline-flex rounded-full bg-amber-500/15 px-2.5 py-1 text-xs font-semibold text-amber-300">已实现未启用</span>;
  }
  return <span className="inline-flex rounded-full bg-slate-500/15 px-2.5 py-1 text-xs font-semibold text-slate-300">计划中</span>;
}

function formatEvidenceTime(timestamp?: number): string {
  if (!timestamp) return '—';
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp * 1000));
}

function diagnosticStatusText(item: AuditDiagnosticLatestItem): string {
  if (!item.usable) {
    return item.run.run_status || item.filter_reason || item.run.status || '无效样本';
  }
  switch (item.run.status) {
    case 'done':
      return '已完成';
    case 'failed_auth':
      return '凭证失败';
    case 'failed_request':
      return '请求失败';
    case 'running':
      return '检测中';
    default:
      return item.run.status || '未知';
  }
}

function diagnosticActionText(item: AuditDiagnosticLatestItem): string {
  if (!item.usable) return '查看无效样本';
  if (item.run.status && item.run.status !== 'done') return '查看失败详情';
  return '查看检测证据';
}

function DiagnosticStatusBadge({ item }: { item: AuditDiagnosticLatestItem }) {
  const status = item.run.status;
  const text = diagnosticStatusText(item);
  if (!item.usable) {
    return <span className="inline-flex rounded-full bg-amber-500/15 px-2.5 py-1 text-xs font-semibold text-amber-300">{text}</span>;
  }
  if (status === 'done') {
    return <span className="inline-flex rounded-full bg-emerald-500/15 px-2.5 py-1 text-xs font-semibold text-emerald-300">{text}</span>;
  }
  if (status === 'failed_auth' || status === 'failed_request') {
    return <span className="inline-flex rounded-full bg-red-500/15 px-2.5 py-1 text-xs font-semibold text-red-300">{text}</span>;
  }
  return <span className="inline-flex rounded-full bg-slate-500/15 px-2.5 py-1 text-xs font-semibold text-slate-300">{text}</span>;
}

function LatestDiagnosticEvidence() {
  const { i18n } = useTranslation();
  const [searchParams] = useSearchParams();
  const langPrefix = LANGUAGE_PATH_MAP[i18n.language as SupportedLanguage];
  const provider = searchParams.get('provider') || undefined;
  const service = searchParams.get('service') || undefined;
  const channel = searchParams.get('channel') || undefined;
  const model = searchParams.get('model') || undefined;
  const filtered = Boolean(provider || service || channel || model);
  const { items, loading, error } = useAuditDiagnosticLatest({
    provider,
    service,
    channel,
    model,
    includeFiltered: true,
    limit: filtered ? 20 : 10,
  });
  const evidenceHref = (runId: string) => (langPrefix ? `/${langPrefix}/detect/compare/${runId}` : `/detect/compare/${runId}`);

  if (loading && items.length === 0) {
    return <div className="rounded-xl border border-default bg-surface p-5 text-sm text-muted">正在加载最近检测证据…</div>;
  }
  if (error) {
    return <div className="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm text-danger">加载最近检测证据失败：{error}</div>;
  }
  if (items.length === 0) {
    return (
      <div className="space-y-3">
        {filtered ? (
          <DiagnosticFilterSummary provider={provider} service={service} channel={channel} model={model} />
        ) : null}
        <div className="rounded-xl border border-default bg-surface p-5 text-sm text-muted">
          {filtered ? '当前通道 / 模型还没有检测历史。完成一次 quick-probe 后会显示在这里。' : '当前还没有检测证据。完成一次 quick-probe 后会显示在这里。'}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {filtered ? (
        <DiagnosticFilterSummary provider={provider} service={service} channel={channel} model={model} />
      ) : null}
      <div className="overflow-x-auto rounded-2xl border border-default bg-surface">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-muted border-b border-default/60">
              <th className="px-3 py-3 font-medium whitespace-nowrap">时间</th>
              <th className="px-3 py-3 font-medium">服务商</th>
              <th className="px-3 py-3 font-medium">通道</th>
              <th className="px-3 py-3 font-medium">模型</th>
              <th className="px-3 py-3 font-medium">状态</th>
              <th className="px-3 py-3 font-medium text-right">分数</th>
              <th className="px-3 py-3 font-medium text-right">证据</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.run.run_id} className="border-b border-default/30 hover:bg-elevated/40 transition-colors last:border-b-0">
                <td className="px-3 py-2.5 font-mono text-xs text-muted whitespace-nowrap">{formatEvidenceTime(item.run.created_at)}</td>
                <td className="px-3 py-2.5 text-primary font-medium">{item.run.provider || '—'}</td>
                <td className="px-3 py-2.5 text-secondary">{item.run.channel || '—'}</td>
                <td className="px-3 py-2.5 font-mono text-xs text-secondary">{item.run.model || item.run.request_model || '—'}</td>
                <td className="px-3 py-2.5">
                  <DiagnosticStatusBadge item={item} />
                  {!item.usable && item.run.run_status_reason ? (
                    <div className="mt-1 max-w-[14rem] text-xs leading-relaxed text-amber-300">{item.run.run_status_reason}</div>
                  ) : null}
                </td>
                <td className="px-3 py-2.5 text-right">
                  {item.score ? (
                    <span
                      className="inline-block min-w-[2.75rem] rounded-md px-2 py-0.5 font-mono font-bold text-[hsl(0_0%_100%)]"
                      style={{ backgroundColor: scoreColor(item.score.overall_score) }}
                    >
                      {item.score.overall_score.toFixed(0)}
                    </span>
                  ) : (
                    <span className="text-muted">—</span>
                  )}
                </td>
                <td className="px-3 py-2.5 text-right">
                  <a
                    href={evidenceHref(item.run.run_id)}
                    className="inline-flex items-center gap-1 text-accent hover:underline whitespace-nowrap"
                  >
                    {diagnosticActionText(item)}
                    <ExternalLink size={12} />
                  </a>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function DiagnosticFilterSummary({
  provider,
  service,
  channel,
  model,
}: {
  provider?: string;
  service?: string;
  channel?: string;
  model?: string;
}) {
  const chips = [
    ['服务商', provider],
    ['服务', service],
    ['通道', channel],
    ['模型', model],
  ].filter(([, value]) => Boolean(value));

  return (
    <div className="rounded-xl border border-default bg-surface px-4 py-3">
      <div className="mb-2 text-sm font-semibold text-primary">当前检测历史筛选</div>
      <div className="flex flex-wrap gap-2">
        {chips.map(([label, value]) => (
          <span key={`${label}-${value}`} className="inline-flex max-w-full items-center gap-1 rounded-full border border-default/70 bg-elevated/60 px-2.5 py-1 text-xs text-secondary">
            <span className="text-muted">{label}</span>
            <span className="truncate font-mono text-primary">{value}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

export default function DetectPage() {
  const { t } = useTranslation();
  const { data: methodology, loading: methodologyLoading, error: methodologyError } = useAuditMethodology();
  const headerStats = useMemo(
    () => ({
      total: methodology?.summary.total_dimensions ?? 0,
      healthy: methodology?.summary.active_count ?? 0,
      issues: Math.max(0, (methodology?.summary.total_dimensions ?? 0) - (methodology?.summary.active_count ?? 0)),
    }),
    [methodology],
  );

  const problems = t('detect.what.items', { returnObjects: true }) as Array<{ t: string; d: string }>;
  const tactics = t('detect.tactics.items', { returnObjects: true }) as Array<{ name: string; how: string }>;
  const steps = t('detect.diy.steps', { returnObjects: true }) as string[];
  const faqs = t('detect.faq.items', { returnObjects: true }) as Array<{ q: string; a: string }>;

  return (
    <>
      <Helmet>
        <title>{t('detect.meta.title')}</title>
        <meta name="description" content={t('detect.meta.description')} />
      </Helmet>

      <div className="min-h-screen bg-page flex flex-col">
        <div className="max-w-7xl mx-auto w-full px-4 pt-4 sm:px-6 lg:px-8">
          <Header stats={headerStats} />
        </div>

        <main className="flex-1 max-w-4xl mx-auto w-full px-4 py-12 sm:py-16">
          {/* Hero */}
          <div className="mb-14">
            <h1 className="text-3xl sm:text-4xl font-bold text-primary mb-4 leading-tight">
              {t('detect.hero.h1')}
            </h1>
            <p className="text-secondary text-lg leading-relaxed max-w-3xl">{t('detect.hero.lead')}</p>
          </div>

          <Section icon={Fingerprint} title="当前检测方法">
            {methodologyLoading ? (
              <div className="rounded-xl border border-default bg-surface p-5 text-sm text-muted">正在加载当前方法版本与样本覆盖情况…</div>
            ) : methodologyError ? (
              <div className="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm text-danger">{methodologyError}</div>
            ) : methodology ? (
              <>
                {methodology.runtime.warning ? (
                  <div className="mb-5 rounded-xl border border-amber-500/30 bg-amber-500/10 p-4 text-sm text-amber-100">
                    <div className="font-semibold text-amber-200">主动探针运行态</div>
                    <div className="mt-1 leading-relaxed">{methodology.runtime.warning}</div>
                    <div className="mt-2 text-xs text-amber-200/90">
                      当前模式：{methodology.runtime.probe_credential_mode} / auth {methodology.runtime.probe_auth_configured ? 'ok' : 'missing'} / user {methodology.runtime.probe_user_configured ? 'ok' : 'missing'}
                    </div>
                  </div>
                ) : null}
                <div className="grid grid-cols-2 gap-4 md:grid-cols-4 mb-5">
                  <MetricCard label="当前版本" value={methodology.summary.version} hint={methodology.summary.weights_hash} />
                  <MetricCard
                    label="已接入维度"
                    value={`${methodology.summary.active_count}/${methodology.summary.total_dimensions}`}
                    hint={`已实现 ${methodology.summary.implemented_count} 维`}
                  />
                  <MetricCard
                    label="当前有效权重"
                    value={`${methodology.summary.active_weight}/${methodology.summary.total_weight}`}
                    hint={`已实现权重 ${methodology.summary.implemented_weight}`}
                  />
                  <MetricCard
                    label="样本覆盖"
                    value={`${methodology.coverage.dimension_runs}`}
                    hint={`usable ${methodology.coverage.done_runs} / 401失败 ${methodology.coverage.failed_auth_runs} / 请求失败 ${methodology.coverage.failed_request_runs}`}
                  />
                </div>
                <div className="mb-5 rounded-xl border border-default bg-surface p-4 text-sm text-secondary">
                  <div>有效样本：{methodology.coverage.done_runs}</div>
                  <div>维度样本：{methodology.coverage.dimension_runs}</div>
                  <div>维度行数：{methodology.coverage.dimension_row_count}</div>
                  <div>已过滤无效样本：{methodology.coverage.filtered_runs}</div>
                </div>
                <div className="rounded-2xl border border-default bg-surface overflow-hidden">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-default/60 text-left text-muted">
                        <th className="px-3 py-3 font-medium">维度</th>
                        <th className="px-3 py-3 font-medium">分组</th>
                        <th className="px-3 py-3 font-medium text-right">权重</th>
                        <th className="px-3 py-3 font-medium">状态</th>
                        <th className="px-3 py-3 font-medium">说明</th>
                      </tr>
                    </thead>
                    <tbody>
                      {methodology.dimensions.map((dimension) => (
                        <tr key={dimension.key} className="border-b border-default/30 align-top last:border-b-0">
                          <td className="px-3 py-3 font-mono text-primary text-xs sm:text-sm">{dimension.key}</td>
                          <td className="px-3 py-3 text-secondary">{dimension.group}</td>
                          <td className="px-3 py-3 text-right font-mono text-primary">{dimension.weight}</td>
                          <td className="px-3 py-3"><DimensionStatusBadge dimension={dimension} /></td>
                          <td className="px-3 py-3 text-secondary leading-relaxed">{dimension.description}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            ) : (
              <div className="rounded-xl border border-default bg-surface p-5 text-sm text-muted">当前还没有可展示的方法数据。</div>
            )}
          </Section>

          <Section icon={ListChecks} title="最近检测证据">
            <LatestDiagnosticEvidence />
          </Section>

          {/* ① 什么是中转站检测 */}
          <Section icon={ShieldQuestion} title={t('detect.what.title')}>
            <p className="text-secondary leading-relaxed mb-5">{t('detect.what.intro')}</p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              {problems.map((p, i) => (
                <div key={i} className="rounded-xl border border-default bg-surface p-4">
                  <h3 className="text-primary font-semibold mb-1">{p.t}</h3>
                  <p className="text-secondary text-sm leading-relaxed">{p.d}</p>
                </div>
              ))}
            </div>
          </Section>

          {/* ② 两层信号 */}
          <Section icon={Fingerprint} title={t('detect.how.title')}>
            <p className="text-secondary leading-relaxed mb-5">{t('detect.how.intro')}</p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
              <div className="rounded-xl border border-default bg-surface p-5">
                <h3 className="text-primary font-semibold mb-2 flex items-center gap-2">
                  <Activity size={16} className="text-accent" />
                  {t('detect.how.availability.t')}
                </h3>
                <p className="text-secondary text-sm leading-relaxed">{t('detect.how.availability.d')}</p>
              </div>
              <div className="rounded-xl border border-default bg-surface p-5">
                <h3 className="text-primary font-semibold mb-2 flex items-center gap-2">
                  <Fingerprint size={16} className="text-accent" />
                  {t('detect.how.quality.t')}
                </h3>
                <p className="text-secondary text-sm leading-relaxed">{t('detect.how.quality.d')}</p>
              </div>
            </div>
            <p className="text-sm text-muted leading-relaxed">{t('detect.how.note')}</p>
          </Section>

          {/* ③ 实时质量排行 */}
          <Section icon={ListChecks} title={t('detect.ranking.title')}>
            <p className="text-secondary leading-relaxed mb-2">{t('detect.ranking.intro')}</p>
            <p className="text-sm text-muted leading-relaxed mb-5">{t('detect.ranking.disclaimer')}</p>
            <QualityRanking />
          </Section>

          {/* ④ 常见掺水手法 */}
          <Section icon={ShieldQuestion} title={t('detect.tactics.title')}>
            <p className="text-secondary leading-relaxed mb-5">{t('detect.tactics.intro')}</p>
            <div className="space-y-3">
              {tactics.map((tac, i) => (
                <div key={i} className="rounded-xl border border-default bg-surface p-4">
                  <h3 className="text-primary font-semibold mb-1">{tac.name}</h3>
                  <p className="text-secondary text-sm leading-relaxed">{tac.how}</p>
                </div>
              ))}
            </div>
            <p className="text-sm text-muted leading-relaxed mt-4">{t('detect.tactics.evidenceNote')}</p>
          </Section>

          {/* ⑤ 怎么自己检测 */}
          <Section icon={ListChecks} title={t('detect.diy.title')}>
            <ol className="space-y-3 mb-6">
              {steps.map((s, i) => (
                <li key={i} className="flex items-start gap-3">
                  <span className="flex items-center justify-center w-6 h-6 rounded-full bg-accent/15 text-accent text-sm font-bold flex-shrink-0 mt-0.5">
                    {i + 1}
                  </span>
                  <span className="text-secondary leading-relaxed">{s}</span>
                </li>
              ))}
            </ol>
          </Section>

          {/* ⑥ FAQ */}
          <Section icon={HelpCircle} title={t('detect.faq.title')}>
            <div className="space-y-4">
              {faqs.map((f, i) => (
                <div key={i} className="rounded-xl border border-default bg-surface p-5">
                  <h3 className="text-primary font-semibold mb-2">{f.q}</h3>
                  <p className="text-secondary text-sm leading-relaxed">{f.a}</p>
                </div>
              ))}
            </div>
          </Section>
        </main>
      </div>
    </>
  );
}
