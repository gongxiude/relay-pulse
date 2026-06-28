import { useEffect, useMemo, useState } from 'react';

import { apiGet } from '../utils/apiClient';
import type { AuditModelStatusItem, AuditModelStatusMeta, AuditModelStatusResponse } from '../types/audit';

interface UseAuditModelStatusArgs {
  provider?: string;
  service?: string;
  channel?: string;
  window?: string;
}

interface UseAuditModelStatusResult {
  items: AuditModelStatusItem[];
  meta: AuditModelStatusMeta | null;
  loading: boolean;
  error: string | null;
}

export function buildAuditModelStatusQuery({
  provider,
  service,
  channel,
  window = '24h',
}: UseAuditModelStatusArgs): string {
  const params = new URLSearchParams();
  if (provider) params.set('provider', provider);
  if (service) params.set('service', service);
  if (channel) params.set('channel', channel);
  params.set('window', window);
  return params.toString();
}

export function useAuditModelStatus({
  provider,
  service,
  channel,
  window = '24h',
}: UseAuditModelStatusArgs): UseAuditModelStatusResult {
  const [items, setItems] = useState<AuditModelStatusItem[]>([]);
  const [meta, setMeta] = useState<AuditModelStatusMeta | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const query = useMemo(
    () => buildAuditModelStatusQuery({ provider, service, channel, window }),
    [provider, service, channel, window],
  );

  useEffect(() => {
    if (!provider || !service || !channel) {
      setItems([]);
      setMeta(null);
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
        setMeta(response?.data?.meta ?? null);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof Error && err.name === 'AbortError') return;
        setMeta(null);
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

  return { items, meta, loading, error };
}

export function useAuditModelStatusSummary({
  window = '24h',
}: {
  window?: string;
} = {}): UseAuditModelStatusResult {
  const [items, setItems] = useState<AuditModelStatusItem[]>([]);
  const [meta, setMeta] = useState<AuditModelStatusMeta | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const query = useMemo(() => buildAuditModelStatusQuery({ window }), [window]);

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();

    setLoading(true);
    setError(null);
    apiGet<AuditModelStatusResponse>(`/api/audit/model-status?${query}`, { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        setItems(Array.isArray(response?.data?.items) ? response.data.items : []);
        setMeta(response?.data?.meta ?? null);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof Error && err.name === 'AbortError') return;
        setMeta(null);
        setError(err instanceof Error ? err.message : '加载审计汇总失败');
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
