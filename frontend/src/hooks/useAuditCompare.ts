import { useEffect, useState } from 'react';

import { apiGet } from '../utils/apiClient';
import type { AuditDiagnosticCompareResponse } from '../types/audit';

interface UseAuditCompareResult {
  data: AuditDiagnosticCompareResponse['data'] | null;
  loading: boolean;
  error: string | null;
}

export function useAuditCompare(runId?: string): UseAuditCompareResult {
  const [data, setData] = useState<AuditDiagnosticCompareResponse['data'] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!runId) {
      setData(null);
      setLoading(false);
      setError(null);
      return;
    }
    let cancelled = false;
    const controller = new AbortController();

    setLoading(true);
    setError(null);
    apiGet<AuditDiagnosticCompareResponse>(`/api/audit/compare/${runId}`, { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        setData(response?.data ?? null);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : '加载对比结果失败');
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [runId]);

  return { data, loading, error };
}
