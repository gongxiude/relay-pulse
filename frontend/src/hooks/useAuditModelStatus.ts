import { useEffect, useMemo, useState } from 'react';

import { apiGet } from '../utils/apiClient';
import type { AuditModelStatusItem, AuditModelStatusResponse } from '../types/audit';

interface UseAuditModelStatusArgs {
  provider?: string;
  service?: string;
  channel?: string;
  window?: string;
}

interface UseAuditModelStatusResult {
  items: AuditModelStatusItem[];
  loading: boolean;
  error: string | null;
}

export function useAuditModelStatus({
  provider,
  service,
  channel,
  window = '24h',
}: UseAuditModelStatusArgs): UseAuditModelStatusResult {
  const [items, setItems] = useState<AuditModelStatusItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const query = useMemo(() => {
    const params = new URLSearchParams();
    if (provider) params.set('provider', provider);
    if (service) params.set('service', service);
    if (channel) params.set('channel', channel);
    params.set('window', window);
    return params.toString();
  }, [provider, service, channel, window]);

  useEffect(() => {
    if (!provider || !service || !channel) {
      setItems([]);
      setLoading(false);
      setError(null);
      return;
    }
    let cancelled = false;
    const controller = new AbortController();

    setLoading(true);
    setError(null);
    apiGet<AuditModelStatusResponse>(`/api/audit/model-status?${query}`, { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        setItems(Array.isArray(response?.data?.items) ? response.data.items : []);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : '加载模型状态来源失败');
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [provider, service, channel, query]);

  return { items, loading, error };
}
