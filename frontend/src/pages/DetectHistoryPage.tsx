import { useMemo } from 'react';
import { Helmet } from 'react-helmet-async';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import { ExternalLink, History } from 'lucide-react';

import { Header } from '../components/Header';
import { useAuditDiagnosticHistory } from '../hooks/useAuditDiagnosticHistory';
import type { AuditDiagnosticLatestItem } from '../types/audit';

function toInt(value: string | null, fallback: number): number {
  if (!value) return fallback;
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback;
}

function formatTime(timestamp?: number): string {
  if (!timestamp) return '--';
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(timestamp * 1000));
}

function statusText(item: AuditDiagnosticLatestItem): string {
  const status = item.run.run_status || item.filter_reason || item.run.status || 'unknown';
  switch (status) {
    case 'done':
      return item.usable ? '有效' : '完成但无效';
    case 'failed_auth':
      return '认证失败';
    case 'failed_request':
      return '请求失败';
    case 'not_done':
      return '未完成';
    default:
      return status;
  }
}

function StatusBadge({ item }: { item: AuditDiagnosticLatestItem }) {
  const text = statusText(item);
  const raw = (item.run.run_status || item.filter_reason || item.run.status || '').toLowerCase();
  let className = 'bg-slate-500/15 text-slate-300';
  if (item.usable) className = 'bg-emerald-500/15 text-emerald-300';
  else if (raw === 'failed_auth' || raw === 'failed_request') className = 'bg-rose-500/15 text-rose-300';
  else if (raw === 'done') className = 'bg-amber-500/15 text-amber-300';
  return <span className={`inline-flex rounded-full px-2.5 py-1 text-xs font-semibold ${className}`}>{text}</span>;
}

function scoreText(item: AuditDiagnosticLatestItem): string {
  if (!item.score) return '--';
  return String(Math.round(item.score.overall_score));
}

function updateOffset(searchParams: URLSearchParams, offset: number): string {
  const next = new URLSearchParams(searchParams);
  next.set('offset', String(Math.max(0, offset)));
  return `?${next.toString()}`;
}

export default function DetectHistoryPage() {
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const provider = searchParams.get('provider') || undefined;
  const service = searchParams.get('service') || undefined;
  const channel = searchParams.get('channel') || undefined;
  const model = searchParams.get('model') || undefined;
  const status = searchParams.get('status') || undefined;
  const limit = toInt(searchParams.get('limit'), 50);
  const offset = toInt(searchParams.get('offset'), 0);

  const { items, meta, loading, error } = useAuditDiagnosticHistory({
    provider,
    service,
    channel,
    model,
    status,
    limit,
    offset,
  });

  const headerStats = useMemo(() => {
    const healthy = items.filter((item) => item.usable).length;
    return {
      total: meta?.total ?? items.length,
      healthy,
      issues: Math.max(0, items.length - healthy),
    };
  }, [items, meta]);

  const detailPath = (runId: string) => {
    const prefixMatch = location.pathname.match(/^\/(en|ru|ja)(\/|$)/);
    const prefix = prefixMatch ? `/${prefixMatch[1]}` : '';
    return `${prefix}/detect/compare/${runId}`;
  };

  return (
    <>
      <Helmet>
        <title>检测历史 - RelayPulse</title>
        <meta name="description" content="按服务商、通道和模型查看检测历史样本。" />
      </Helmet>
      <div className="min-h-screen bg-page text-primary">
        <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
          <Header stats={headerStats} />

          <section className="mb-6 rounded-2xl border border-default/70 bg-surface/55 px-5 py-5">
            <div className="flex flex-wrap items-center gap-3">
              <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-accent/10 text-accent">
                <History size={20} />
              </span>
              <div>
                <h1 className="text-2xl font-bold text-primary">检测历史</h1>
                <p className="mt-1 text-sm text-secondary">当前页面展示该通道、该模型的检测样本；每条样本可进入实际检测情况。</p>
              </div>
            </div>
            <div className="mt-4 flex flex-wrap gap-2 text-xs">
              <span className="rounded-full border border-default/70 px-2.5 py-1 text-secondary">服务商 {provider || '--'}</span>
              <span className="rounded-full border border-default/70 px-2.5 py-1 text-secondary">服务 {service || '--'}</span>
              <span className="rounded-full border border-default/70 px-2.5 py-1 text-secondary">通道 {channel || '--'}</span>
              <span className="rounded-full border border-default/70 px-2.5 py-1 text-secondary">模型 {model || '--'}</span>
            </div>
          </section>

          {loading && items.length === 0 ? (
            <div className="rounded-xl border border-default bg-surface p-5 text-sm text-muted">正在加载检测历史...</div>
          ) : error ? (
            <div className="rounded-xl border border-danger/30 bg-danger/5 p-5 text-sm text-danger">{error}</div>
          ) : items.length === 0 ? (
            <div className="rounded-xl border border-default bg-surface p-5 text-sm text-muted">当前通道和模型还没有检测历史。</div>
          ) : (
            <>
              <div className="mb-3 text-sm text-secondary">
                共 {meta?.total ?? items.length} 条，当前显示 {items.length} 条
              </div>
              <div className="overflow-x-auto rounded-2xl border border-default bg-surface">
                <table className="w-full min-w-[980px] text-sm">
                  <thead>
                    <tr className="border-b border-default/60 text-left text-muted">
                      <th className="px-3 py-3 font-medium">时间</th>
                      <th className="px-3 py-3 font-medium">状态</th>
                      <th className="px-3 py-3 font-medium">模型</th>
                      <th className="px-3 py-3 font-medium">Run ID</th>
                      <th className="px-3 py-3 text-right font-medium">分数</th>
                      <th className="px-3 py-3 text-right font-medium">实际情况</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((item) => (
                      <tr key={item.run.run_id} className="border-b border-default/30 last:border-b-0 hover:bg-elevated/40">
                        <td className="px-3 py-3 font-mono text-xs text-muted">{formatTime(item.run.created_at)}</td>
                        <td className="px-3 py-3"><StatusBadge item={item} /></td>
                        <td className="px-3 py-3 font-mono text-xs text-secondary">{item.run.model || item.run.request_model || '--'}</td>
                        <td className="px-3 py-3 font-mono text-xs text-muted">{item.run.run_id}</td>
                        <td className="px-3 py-3 text-right font-mono text-primary">{scoreText(item)}</td>
                        <td className="px-3 py-3 text-right">
                          <Link to={detailPath(item.run.run_id)} className="inline-flex items-center gap-1 text-accent hover:underline">
                            查看实际情况
                            <ExternalLink size={12} />
                          </Link>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div className="mt-4 flex items-center justify-end gap-2">
                {offset > 0 ? (
                  <Link to={updateOffset(searchParams, Math.max(0, offset - limit))} className="rounded-lg border border-default px-3 py-1.5 text-sm text-secondary hover:text-primary">
                    上一页
                  </Link>
                ) : null}
                {meta?.next_offset != null ? (
                  <Link to={updateOffset(searchParams, meta.next_offset)} className="rounded-lg border border-default px-3 py-1.5 text-sm text-secondary hover:text-primary">
                    下一页
                  </Link>
                ) : null}
              </div>
            </>
          )}
        </div>
      </div>
    </>
  );
}
