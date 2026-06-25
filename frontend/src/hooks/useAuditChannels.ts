import { useEffect, useState } from 'react';

import { apiGet } from '../utils/apiClient';
import type { AuditChannelSnapshot, AuditChannelsResponse } from '../types/audit';

interface UseAuditChannelsResult {
  channels: AuditChannelSnapshot[];
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

export function useAuditChannels(): UseAuditChannelsResult {
  const [channels, setChannels] = useState<AuditChannelSnapshot[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();

    setLoading(true);
    setError(null);

    apiGet<AuditChannelsResponse>('/api/audit/channels', { signal: controller.signal })
      .then((response) => {
        if (cancelled) return;
        setChannels(Array.isArray(response?.data) ? response.data : []);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof Error && err.name === 'AbortError') return;
        setError(err instanceof Error ? err.message : '加载渠道快照失败');
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [reloadToken]);

  return {
    channels,
    loading,
    error,
    refetch: () => setReloadToken((token) => token + 1),
  };
}
