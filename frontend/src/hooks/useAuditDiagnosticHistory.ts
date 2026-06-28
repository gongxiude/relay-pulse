import { useEffect, useMemo, useState } from 'react';

import { apiGet } from '../utils/apiClient';
import type { AuditDiagnosticHistoryMeta, AuditDiagnosticHistoryResponse, AuditDiagnosticLatestItem } from '../types/audit';

interface UseAuditDiagnosticHistoryArgs {
  provider?: string;
  service?: string;
  channel?: string;
  model?: string;
  status?: string;
  limit?: number;
  offset?: number;
}

interface UseAuditDiagnosticHistoryResult {
  items: AuditDiagnosticLatestItem[];
  meta: AuditDiagnosticHistoryMeta | null;
  loading: boolean;
  error: string | null;
}

export function useAuditDiagnosticHistory({
  provider,
  service,
  channel,
  model,
  status,
  limit = 50,
  offset = 0,
}: UseAuditDiagnosticHistoryArgs): UseAuditDiagnosticHistoryResult {
  const [items, setItems] = useState<AuditDiagnosticLatestItem[]>([]);
  const [meta, setMeta] = useState<AuditDiagnosticHistoryMeta | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const query = useMemo(() => {
    const params = new URLSearchParams();
    if (provider) params.set('provider', provider);
    if (service) params.set('service', service);
    if (channel) params.set('channel', channel);
    if (model) params.set('model', model);
    if (status) params.set('status', status);
    params.set('limit', String(limit));
    params.set('offset', String(offset));
    return params.toString();
  }, [provider, service, channel, model, status, limit, offset]);

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();

    setLoading(true);
    setError(null);
    apiGet<AuditDiagnosticHistoryResponse>(`/api/audit/diagnostics/history?${query}`, { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        setItems(Array.isArray(response?.data?.items) ? response.data.items : []);
        setMeta(response?.data?.meta ?? null);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : '加载检测历史失败');
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [query]);

  return { items, meta, loading, error };
}
