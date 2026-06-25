import { useEffect, useState } from 'react';

import { apiGet } from '../utils/apiClient';
import type { AuditMethodologyResponse } from '../types/audit';

interface UseAuditMethodologyResult {
  data: AuditMethodologyResponse['data'] | null;
  loading: boolean;
  error: string | null;
}

export function useAuditMethodology(): UseAuditMethodologyResult {
  const [data, setData] = useState<AuditMethodologyResponse['data'] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();

    setLoading(true);
    setError(null);
    apiGet<AuditMethodologyResponse>('/api/audit/methodology', { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        setData(response?.data ?? null);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : '加载检测方法失败');
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
