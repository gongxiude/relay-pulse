import { useEffect, useMemo, useState } from 'react';

import { apiGet } from '../utils/apiClient';
import type { AuditDiagnosticLatestItem, AuditDiagnosticLatestResponse } from '../types/audit';

interface UseAuditDiagnosticLatestArgs {
  provider?: string;
  service?: string;
  channel?: string;
  model?: string;
  includeFiltered?: boolean;
  limit?: number;
}

interface UseAuditDiagnosticLatestResult {
  items: AuditDiagnosticLatestItem[];
  loading: boolean;
  error: string | null;
}

export function useAuditDiagnosticLatest({
  provider,
  service,
  channel,
  model,
  includeFiltered = false,
  limit = 10,
}: UseAuditDiagnosticLatestArgs): UseAuditDiagnosticLatestResult {
  const [items, setItems] = useState<AuditDiagnosticLatestItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const query = useMemo(() => {
    const params = new URLSearchParams();
    if (provider) params.set('provider', provider);
    if (service) params.set('service', service);
    if (channel) params.set('channel', channel);
    if (model) params.set('model', model);
    if (includeFiltered) params.set('include_filtered', '1');
    params.set('limit', String(limit));
    return params.toString();
  }, [provider, service, channel, model, includeFiltered, limit]);

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();

    setLoading(true);
    setError(null);
    apiGet<AuditDiagnosticLatestResponse>(`/api/audit/diagnostics/latest?${query}`, { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        setItems(Array.isArray(response?.data?.items) ? response.data.items : []);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : '加载最近诊断结果失败');
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

  return { items, loading, error };
}
