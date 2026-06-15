import { useState, useEffect, useCallback, useRef } from 'react';
import { API_BASE_URL } from '../constants';

// localStorage key
const STORAGE_KEY = 'relay-pulse.announcements.v1';

// 轮询间隔（毫秒）
const POLL_INTERVAL_MS = 120_000; // 2 分钟

// API 响应类型
export interface AnnouncementItem {
  id: string;
  number: number;
  title: string;
  url: string;
  createdAt: string;
  author?: string;
}

export interface AnnouncementsResponse {
  enabled: boolean;
  source: {
    provider: string;
    repo: string;
    category: string;
    discussionsUrl: string;
  };
  window: {
    hours: number;
    startAt: string;
    endAt: string;
  };
  fetch: {
    fetchedAt: string;
    stale: boolean;
    ttlSeconds: number;
  };
  latest?: AnnouncementItem;
  items: AnnouncementItem[];
  version: string;
  apiMaxAge: number;
}

// localStorage 状态
interface AnnouncementsState {
  dismissedUntilVersion: string | null;
  dismissedAt: string | null;
}

// 从 localStorage 加载状态。纯函数（只读模块级 STORAGE_KEY），置于组件外避免
// 在 useState 初始化器中先用后声明（react-hooks/immutability）。
function loadState(): AnnouncementsState {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored);
      return {
        dismissedUntilVersion: parsed.dismissedUntilVersion || null,
        dismissedAt: parsed.dismissedAt || null,
      };
    }
  } catch {
    // ignore
  }
  return {
    dismissedUntilVersion: null,
    dismissedAt: null,
  };
}

// Hook 返回类型
export interface UseAnnouncementsReturn {
  // 数据
  data: AnnouncementsResponse | null;
  loading: boolean;
  error: string | null;

  // 状态
  hasUnread: boolean;           // 是否有未读公告
  shouldShowBanner: boolean;    // 是否应该显示 Banner

  // 操作
  dismiss: () => void;          // 关闭/标记已读
  refresh: () => Promise<void>; // 手动刷新
}

/**
 * 公告通知 Hook
 *
 * 功能：
 * - 轮询 /api/announcements 获取公告数据
 * - 通过 localStorage 管理已读状态
 * - 判断是否显示 Banner 和角标
 *
 * @param enabled 是否启用公告功能（默认 true，截图模式下传 false 跳过请求）
 */
export function useAnnouncements(enabled: boolean = true): UseAnnouncementsReturn {
  const [data, setData] = useState<AnnouncementsResponse | null>(null);
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState<string | null>(null);
  const [state, setState] = useState<AnnouncementsState>(() => loadState());

  const abortControllerRef = useRef<AbortController | null>(null);
  const pollingRef = useRef<number | null>(null);

  // 保存状态到 localStorage
  function saveState(newState: AnnouncementsState) {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(newState));
    } catch {
      // ignore
    }
    setState(newState);
  }

  // 获取公告数据
  const fetchAnnouncements = useCallback(async () => {
    // 取消之前的请求
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    const controller = new AbortController();
    abortControllerRef.current = controller;

    try {
      const response = await fetch(`${API_BASE_URL}/api/announcements`, {
        signal: controller.signal,
        headers: {
          'Accept': 'application/json',
          'Accept-Encoding': 'gzip',
        },
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      const result: AnnouncementsResponse = await response.json();
      setData(result);
      setError(null);

      // 如果功能被禁用，停止轮询
      if (!result.enabled) {
        if (pollingRef.current) {
          clearInterval(pollingRef.current);
          pollingRef.current = null;
        }
      }
    } catch (err) {
      if ((err as Error).name !== 'AbortError') {
        setError((err as Error).message);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  // 关闭/标记已读
  const dismiss = useCallback(() => {
    if (data?.version) {
      saveState({
        dismissedUntilVersion: data.version,
        dismissedAt: new Date().toISOString(),
      });
    }
  }, [data?.version]);

  // 手动刷新（enabled=false 时不执行）
  const refresh = useCallback(async () => {
    if (!enabled) return;
    setLoading(true);
    await fetchAnnouncements();
  }, [enabled, fetchAnnouncements]);

  // 判断是否有未读公告
  // 条件：功能启用 + 有版本号 + 有公告条目 + 有最新公告（与 Banner 显示条件一致）
  const hasUnread = (() => {
    if (!data?.enabled || !data?.version || data.items.length === 0 || !data.latest) {
      return false;
    }
    if (!state.dismissedUntilVersion) {
      return true;
    }
    // 比较版本：version 格式为 "2026-01-11T10:00:00Z#123"
    return data.version > state.dismissedUntilVersion;
  })();

  // 判断是否应该显示 Banner
  const shouldShowBanner = hasUnread;

  // 初始加载和轮询（enabled=false 时跳过，用于截图模式等场景）
  useEffect(() => {
    if (!enabled) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- 禁用态收尾置 loading=false 为有意（截图模式跳过请求）
      setLoading(false);
      return;
    }

    fetchAnnouncements();

    // 页面可见时轮询
    const startPolling = () => {
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
      }
      pollingRef.current = window.setInterval(fetchAnnouncements, POLL_INTERVAL_MS);
    };

    const stopPolling = () => {
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    };

    const handleVisibilityChange = () => {
      if (document.hidden) {
        stopPolling();
      } else {
        fetchAnnouncements(); // 页面重新可见时立即刷新
        startPolling();
      }
    };

    // 启动轮询
    startPolling();

    // 监听页面可见性
    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      stopPolling();
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, [enabled, fetchAnnouncements]);

  return {
    data,
    loading,
    error,
    hasUnread,
    shouldShowBanner,
    dismiss,
    refresh,
  };
}
