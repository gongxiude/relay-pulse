import { Helmet } from 'react-helmet-async';
import { useMemo } from 'react';
import { useLocation, useParams } from 'react-router-dom';

import { Footer } from '../components/Footer';
import { Header } from '../components/Header';
import { useAuditCompare } from '../hooks/useAuditCompare';
import { useSeoMeta } from '../hooks/useSeoMeta';

function formatMetric(value?: number | null) {
  if (value == null || Number.isNaN(value)) return '—';
  return `${Math.round(value)} ms`;
}

function scoreBadgeClass(score?: number | null) {
  if (score == null) return 'bg-slate-500/15 text-slate-300';
  if (score < 50) return 'bg-rose-500/15 text-rose-300';
  if (score < 70) return 'bg-amber-500/15 text-amber-300';
  if (score < 85) return 'bg-lime-500/15 text-lime-300';
  return 'bg-emerald-500/15 text-emerald-300';
}

export default function DetectComparePage() {
  const { runId } = useParams<{ runId: string }>();
  const location = useLocation();
  const seo = useSeoMeta({ pathname: location.pathname, language: 'zh-CN' });
  const { data, loading, error } = useAuditCompare(runId);

  const headerStats = useMemo(() => ({
    total: data?.dimensions.length ?? 0,
    healthy: data?.dimensions.filter((dimension) => dimension.status === 'pass').length ?? 0,
    issues: data?.dimensions.filter((dimension) => dimension.status !== 'pass').length ?? 0,
  }), [data]);

  return (
    <>
      <Helmet>
        <html lang={seo.htmlLang} />
        <title>检测对比详情</title>
        <meta name="description" content="候选通道与官方基线的本地审计对比详情。" />
      </Helmet>

      <div className="min-h-screen bg-page text-primary flex flex-col">
        <div className="max-w-7xl mx-auto w-full px-4 pt-4 sm:px-6 lg:px-8">
          <Header stats={headerStats} />
        </div>

        <main className="flex-1 max-w-7xl mx-auto w-full px-4 py-8 sm:px-6 lg:px-8">
          <section className="mb-6">
            <h1 className="text-3xl font-bold tracking-tight">检测对比详情</h1>
            <p className="mt-2 text-secondary">本页展示候选通道与最近一次官方基线的步骤对照、维度分和证据摘要。</p>
          </section>

          {loading ? (
            <div className="rounded-2xl border border-default bg-surface p-8 text-center text-muted">正在加载对比结果…</div>
          ) : error ? (
            <div className="rounded-2xl border border-danger/30 bg-danger/5 p-8 text-center text-danger">{error}</div>
          ) : !data ? (
            <div className="rounded-2xl border border-default bg-surface p-8 text-center text-muted">没有可展示的对比结果。</div>
          ) : (
            <>
              <section className="grid gap-4 md:grid-cols-3 mb-6">
                <div className="rounded-xl border border-default bg-surface p-4">
                  <div className="text-sm text-muted mb-1">候选通道</div>
                  <div className="text-lg font-semibold">{data.candidate.run.provider}</div>
                  <div className="text-sm text-secondary mt-1">{data.candidate.run.channel} / {data.candidate.run.model}</div>
                </div>
                <div className="rounded-xl border border-default bg-surface p-4">
                  <div className="text-sm text-muted mb-1">官方基线</div>
                  <div className="text-lg font-semibold">{data.baseline?.run.provider || '未命中基线'}</div>
                  <div className="text-sm text-secondary mt-1">{data.baseline?.run.channel || '—'} / {data.baseline?.run.model || '—'}</div>
                </div>
                <div className="rounded-xl border border-default bg-surface p-4">
                  <div className="text-sm text-muted mb-1">总分</div>
                  <div className={`inline-flex rounded-full px-3 py-1 text-lg font-bold ${scoreBadgeClass(data.summary.overall_score)}`}>
                    {Math.round(data.summary.overall_score)}
                  </div>
                  <div className="text-sm text-secondary mt-1">
                    {data.group.methodology_version || 'quick-probe-v1'} / active weight {data.summary.active_weight}
                  </div>
                </div>
              </section>

              <section className="mb-6 rounded-2xl border border-default bg-surface overflow-hidden">
                <div className="border-b border-default/60 px-4 py-3 font-semibold">维度评分</div>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-default/50 text-left text-muted">
                        <th className="px-4 py-3 font-medium">维度</th>
                        <th className="px-4 py-3 font-medium text-right">权重</th>
                        <th className="px-4 py-3 font-medium text-right">分数</th>
                        <th className="px-4 py-3 font-medium">状态</th>
                        <th className="px-4 py-3 font-medium">原因</th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.dimensions.map((dimension) => (
                        <tr key={dimension.dimension_key} className="border-b border-default/30 align-top last:border-b-0">
                          <td className="px-4 py-3 font-mono text-xs sm:text-sm">{dimension.dimension_key}</td>
                          <td className="px-4 py-3 text-right font-mono">{dimension.weight}</td>
                          <td className="px-4 py-3 text-right font-mono">{dimension.score.toFixed(1)}</td>
                          <td className="px-4 py-3">{dimension.status}</td>
                          <td className="px-4 py-3 text-secondary">{dimension.reason}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>

              <section className="rounded-2xl border border-default bg-surface overflow-hidden">
                <div className="border-b border-default/60 px-4 py-3 font-semibold">步骤对照</div>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-default/50 text-left text-muted">
                        <th className="px-4 py-3 font-medium">步骤</th>
                        <th className="px-4 py-3 font-medium">候选结果</th>
                        <th className="px-4 py-3 font-medium">候选时延</th>
                        <th className="px-4 py-3 font-medium">基线结果</th>
                        <th className="px-4 py-3 font-medium">基线时延</th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.steps.map((step) => (
                        <tr key={step.step_index} className="border-b border-default/30 align-top last:border-b-0">
                          <td className="px-4 py-3">
                            <div className="font-semibold">{step.step_name || `step_${step.step_index}`}</div>
                            <div className="text-xs text-muted mt-1">#{step.step_index}</div>
                          </td>
                          <td className="px-4 py-3 text-secondary">
                            <div>{step.candidate.response_preview || step.candidate.result_summary || '—'}</div>
                            {step.candidate.error_message && <div className="text-danger text-xs mt-1">{step.candidate.error_message}</div>}
                          </td>
                          <td className="px-4 py-3 text-secondary">
                            <div>TTFB {formatMetric(step.candidate.execution.http_ttfb_ms)}</div>
                            <div>TTFT {formatMetric(step.candidate.execution.ttft_ms)}</div>
                          </td>
                          <td className="px-4 py-3 text-secondary">
                            <div>{step.baseline?.response_preview || step.baseline?.result_summary || '—'}</div>
                            {step.baseline?.error_message && <div className="text-danger text-xs mt-1">{step.baseline.error_message}</div>}
                          </td>
                          <td className="px-4 py-3 text-secondary">
                            <div>TTFB {formatMetric(step.baseline?.execution.http_ttfb_ms)}</div>
                            <div>TTFT {formatMetric(step.baseline?.execution.ttft_ms)}</div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            </>
          )}
        </main>

        <div className="max-w-7xl mx-auto w-full px-4 sm:px-6 lg:px-8">
          <Footer />
        </div>
      </div>
    </>
  );
}
