/**
 * 收藏管理 Hook
 *
 * 功能：
 * - 版本化 localStorage 持久化
 * - 启动时去重/清理无效项
 * - 监听 storage 事件同步跨标签页
 * - localStorage 不可用时降级为内存状态
 */

import { useCallback, useEffect, useRef, useState } from 'react';

// ============================================================================
// 类型定义
// ============================================================================

export interface FavoritesStorage {
  version: 1;
  items: string[];
}

export interface UseFavoritesReturn {
  /** 收藏项集合 */
  favorites: Set<string>;
  /** 检查某项是否已收藏 */
  isFavorite: (id: string) => boolean;
  /** 切换收藏状态 */
  toggleFavorite: (id: string) => void;
  /** 清理无效收藏：移除不在 validIds 中的项 */
  cleanupMissingFavorites: (validIds: Set<string>) => void;
  /** 清空所有收藏 */
  clearFavorites: () => void;
  /** 收藏数量 */
  count: number;
}

// ============================================================================
// 常量
// ============================================================================

const STORAGE_KEY = 'relay-pulse-favorites';
const STORAGE_VERSION = 1 as const;

// ============================================================================
// 工具函数
// ============================================================================

/**
 * 规范化 id：去除空白，返回 null 表示无效
 */
function normalizeId(id: unknown): string | null {
  if (typeof id !== 'string') return null;
  const trimmed = id.trim();
  return trimmed || null;
}

/**
 * 解析并清理收藏列表
 */
function parseAndSanitize(raw: string | null): {
  favorites: Set<string>;
  needsCleanup: boolean;
} {
  if (!raw) {
    return { favorites: new Set<string>(), needsCleanup: false };
  }

  try {
    const parsed: unknown = JSON.parse(raw);

    // 检查版本化结构
    if (
      typeof parsed === 'object' &&
      parsed !== null &&
      'version' in parsed &&
      'items' in parsed &&
      (parsed as FavoritesStorage).version === STORAGE_VERSION
    ) {
      const items = (parsed as FavoritesStorage).items;
      if (!Array.isArray(items)) {
        return { favorites: new Set<string>(), needsCleanup: true };
      }

      const favorites = new Set<string>();
      let needsCleanup = false;

      for (const item of items) {
        const id = normalizeId(item);
        if (!id) {
          needsCleanup = true;
          continue;
        }
        // 检查重复
        if (favorites.has(id)) {
          needsCleanup = true;
          continue;
        }
        favorites.add(id);
      }

      return { favorites, needsCleanup };
    }

    // 版本不匹配或结构错误
    return { favorites: new Set<string>(), needsCleanup: true };
  } catch {
    // JSON 解析失败
    return { favorites: new Set<string>(), needsCleanup: true };
  }
}

/**
 * 序列化收藏列表
 */
function serialize(favorites: Set<string>): string {
  const payload: FavoritesStorage = {
    version: STORAGE_VERSION,
    items: Array.from(favorites),
  };
  return JSON.stringify(payload);
}

/**
 * 从 localStorage 读取收藏
 */
function readFromStorage(): {
  favorites: Set<string>;
  needsCleanup: boolean;
  available: boolean;
} {
  if (typeof window === 'undefined') {
    return { favorites: new Set<string>(), needsCleanup: false, available: false };
  }

  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    const { favorites, needsCleanup } = parseAndSanitize(raw);
    return { favorites, needsCleanup, available: true };
  } catch {
    // localStorage 不可用（隐私模式/安全策略）
    return { favorites: new Set<string>(), needsCleanup: false, available: false };
  }
}

/**
 * 写入 localStorage
 */
function writeToStorage(favorites: Set<string>): boolean {
  try {
    localStorage.setItem(STORAGE_KEY, serialize(favorites));
    return true;
  } catch {
    return false;
  }
}

/**
 * 清除 localStorage
 */
function clearStorage(): boolean {
  try {
    localStorage.removeItem(STORAGE_KEY);
    return true;
  } catch {
    return false;
  }
}

// ============================================================================
// Hook
// ============================================================================

export function useFavorites(): UseFavoritesReturn {
  // 初始化：读取 localStorage
  const [initState] = useState(() => readFromStorage());
  const [favorites, setFavorites] = useState<Set<string>>(initState.favorites);

  // 用 ref 跟踪 storage 可用性，避免在 effect 中调用 setState
  const storageAvailableRef = useRef(initState.available);
  // 用于跳过跨标签页同步触发的持久化
  const skipNextPersistRef = useRef(false);
  // 用于跟踪是否需要清理
  const needsCleanupRef = useRef(initState.needsCleanup);

  // 持久化到 localStorage
  useEffect(() => {
    if (!storageAvailableRef.current) return;

    // 跳过跨标签页同步触发的持久化
    if (skipNextPersistRef.current) {
      skipNextPersistRef.current = false;
      return;
    }

    // 首次渲染时，如果需要清理则写入
    // 后续渲染时，正常写入
    // 不预置初值：成功分支会在读取前赋值，catch 分支 return，初值恒被覆盖（no-useless-assignment）。
    let hasExistingData: boolean;
    try {
      hasExistingData = localStorage.getItem(STORAGE_KEY) !== null;
    } catch {
      // localStorage 不可用
      storageAvailableRef.current = false;
      return;
    }

    if (needsCleanupRef.current || favorites.size > 0 || hasExistingData) {
      const success = writeToStorage(favorites);
      if (!success) {
        storageAvailableRef.current = false;
      }
      needsCleanupRef.current = false;
    }
  }, [favorites]);

  // 跨标签页同步
  useEffect(() => {
    if (typeof window === 'undefined') return;

    const handleStorage = (event: StorageEvent) => {
      if (event.key !== STORAGE_KEY) return;

      const { favorites: newFavorites } = parseAndSanitize(event.newValue);
      skipNextPersistRef.current = true;
      setFavorites(newFavorites);
      storageAvailableRef.current = true;
    };

    window.addEventListener('storage', handleStorage);
    return () => window.removeEventListener('storage', handleStorage);
  }, []);

  const isFavorite = useCallback(
    (id: string): boolean => {
      const normalized = normalizeId(id);
      return normalized ? favorites.has(normalized) : false;
    },
    [favorites]
  );

  const toggleFavorite = useCallback((id: string): void => {
    const normalized = normalizeId(id);
    if (!normalized) return;

    setFavorites((prev) => {
      const next = new Set(prev);
      if (next.has(normalized)) {
        next.delete(normalized);
      } else {
        next.add(normalized);
      }
      return next;
    });
  }, []);

  // 清理无效收藏：移除不在 validIds 中的项
  // 用于清理已删除的监控项对应的收藏
  const cleanupMissingFavorites = useCallback((validIds: Set<string>): void => {
    setFavorites((prev) => {
      // 无收藏，无需清理
      if (prev.size === 0) return prev;

      let changed = false;
      const next = new Set<string>();

      for (const id of prev) {
        if (validIds.has(id)) {
          next.add(id);
        } else {
          changed = true;
        }
      }

      // 只有真正发生变化时才返回新 Set
      return changed ? next : prev;
    });
  }, []);

  const clearFavorites = useCallback((): void => {
    setFavorites(new Set<string>());
    if (storageAvailableRef.current) {
      clearStorage();
    }
  }, []);

  return {
    favorites,
    isFavorite,
    toggleFavorite,
    cleanupMissingFavorites,
    clearFavorites,
    count: favorites.size,
  };
}
