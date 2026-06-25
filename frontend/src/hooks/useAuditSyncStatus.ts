import { useEffect, useState } from 'react';

import { apiGet } from '../utils/apiClient';
import type { AuditSyncStatusResponse } from '../types/audit';

interface UseAuditSyncStatusResult {
  data: AuditSyncStatusResponse['data'] | null;
  loading: boolean;
  error: string | null;
}

export function useAuditSyncStatus(): UseAuditSyncStatusResult {
  const [data, setData] = useState<AuditSyncStatusResponse['data'] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();

    setLoading(true);
    setError(null);
    apiGet<AuditSyncStatusResponse>('/api/audit/newapi/sync/status', { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        setData(response?.data ?? null);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : '加载同步状态失败');
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, []);

  return { data, loading, error };
}
