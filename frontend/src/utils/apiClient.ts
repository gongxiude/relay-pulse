import { API_BASE_URL } from '../constants';

/** apiGet / apiPost 的可选参数 */
export interface ApiRequestOptions {
  signal?: AbortSignal;
  headers?: HeadersInit;
  /** 透传给 fetch 的额外选项（如 cache: 'no-store'） */
  fetchOptions?: Omit<RequestInit, 'method' | 'body' | 'headers' | 'signal'>;
}

interface ApiErrorOptions {
  status?: number;
  code?: string;
  data?: Record<string, unknown>;
}

interface ParsedError {
  message?: string;
  code?: string;
  raw?: Record<string, unknown>;
}

/** API 请求失败时抛出的错误类型 */
export class ApiError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly data?: Record<string, unknown>;

  constructor(message: string, options: ApiErrorOptions = {}) {
    super(message);
    this.name = 'ApiError';
    this.status = options.status ?? 0;
    this.code = options.code;
    this.data = options.data;
  }
}

function buildApiUrl(path: string): string {
  // 绝对 URL 直接使用（如 notifier 服务地址）
  if (/^https?:\/\//i.test(path)) {
    return path;
  }

  const base = API_BASE_URL.replace(/\/+$/, '');
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  return `${base}${normalizedPath}`;
}

/**
 * 解析后端错误响应体，兼容两种格式：
 * - 旧版: { "error": "string" }
 * - 新版: { "error": { "code": "...", "message": "...", "request_id": "..." } }
 */
function parseErrorPayload(errorText: string): ParsedError {
  if (!errorText) return {};

  try {
    const parsed = JSON.parse(errorText) as {
      error?: string | { code?: unknown; message?: unknown };
      message?: unknown;
    };

    if (typeof parsed.error === 'string') {
      return { message: parsed.error };
    }

    if (parsed.error && typeof parsed.error === 'object') {
      return {
        code: typeof parsed.error.code === 'string' ? parsed.error.code : undefined,
        message: typeof parsed.error.message === 'string' ? parsed.error.message : undefined,
        raw: parsed as Record<string, unknown>,
      };
    }

    if (typeof parsed.message === 'string') {
      return { message: parsed.message };
    }
  } catch {
    // 非 JSON 响应体。网关/CDN 的错误页（如 Cloudflare 502/504、挑战页）是整页
    // HTML，绝不能当作错误消息直接渲染给用户；超长 body 同理。仅保留简短纯文本错误。
    const trimmed = errorText.trim();
    if (trimmed.startsWith('<') || trimmed.length > 300) return {};
    return { message: trimmed };
  }

  return {};
}

/**
 * 从服务端错误响应文本中提取用户可见消息。
 * 兼容新旧两种错误格式；无法提取出有意义消息时（如 HTML 网关错误页）回退到 fallback。
 */
export function extractErrorMessage(errorText: string, fallback: string): string {
  if (!errorText) return fallback;
  const { message } = parseErrorPayload(errorText);
  return message || fallback;
}

/** 网关瞬断状态码：源站重建/隧道抖动时短暂出现，对幂等请求可安全重试一次 */
const GATEWAY_BLIP_STATUSES = new Set([502, 503, 504]);
const RETRY_DELAY_MS = 400;

/** 按状态码给出干净的用户可见兜底文案（避免暴露网关 HTML 错误页） */
function statusFallback(status: number): string {
  if (GATEWAY_BLIP_STATUSES.has(status)) {
    return `服务暂时不可用（${status}），请稍后重试`;
  }
  if (status === 429) return '请求过于频繁（429），请稍后重试';
  return `请求失败 (${status})`;
}

function delay(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function performRequest<T>(path: string, init: RequestInit): Promise<T> {
  try {
    const response = await fetch(buildApiUrl(path), init);

    if (!response.ok) {
      const errorText = await response.text();
      const parsed = parseErrorPayload(errorText);

      throw new ApiError(
        extractErrorMessage(errorText, statusFallback(response.status)),
        { status: response.status, code: parsed.code, data: parsed.raw },
      );
    }

    const text = await response.text();
    if (!text) return undefined as T;

    try {
      return JSON.parse(text) as T;
    } catch {
      return text as T;
    }
  } catch (error) {
    if (error instanceof ApiError) throw error;
    if (error instanceof Error && error.name === 'AbortError') throw error;

    const message = error instanceof Error ? error.message : '网络请求失败';
    throw new ApiError(message, { status: 0, code: 'network_error' });
  }
}

async function request<T>(path: string, init: RequestInit): Promise<T> {
  // 仅对幂等方法（GET/HEAD）在网关瞬断（502/503/504）或网络错误（status 0）时静默重试一次，
  // 以吸收源站重建/隧道抖动造成的短暂不可用；非幂等请求与业务错误（4xx/5xx 应用错误）不重试。
  const method = (init.method ?? 'GET').toUpperCase();
  const idempotent = method === 'GET' || method === 'HEAD';
  const maxAttempts = idempotent ? 2 : 1;

  let lastError: unknown;
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      return await performRequest<T>(path, init);
    } catch (error) {
      lastError = error;
      const retriable =
        error instanceof ApiError &&
        (error.status === 0 || GATEWAY_BLIP_STATUSES.has(error.status));
      if (attempt < maxAttempts && retriable) {
        await delay(RETRY_DELAY_MS);
        continue;
      }
      throw error;
    }
  }
  throw lastError;
}

/** 发起 GET 请求并解析 JSON 响应 */
export function apiGet<T>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  return request<T>(path, {
    ...options.fetchOptions,
    method: 'GET',
    headers: options.headers,
    signal: options.signal,
  });
}

/** 发起 POST 请求，自动序列化 body 为 JSON */
export function apiPost<T>(path: string, body: unknown, options: ApiRequestOptions = {}): Promise<T> {
  const headers = new Headers(options.headers);
  if (!headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  return request<T>(path, {
    ...options.fetchOptions,
    method: 'POST',
    headers,
    body: JSON.stringify(body),
    signal: options.signal,
  });
}

/** 发起 PUT 请求，自动序列化 body 为 JSON */
export function apiPut<T>(path: string, body: unknown, options: ApiRequestOptions = {}): Promise<T> {
  const headers = new Headers(options.headers);
  if (!headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  return request<T>(path, {
    ...options.fetchOptions,
    method: 'PUT',
    headers,
    body: JSON.stringify(body),
    signal: options.signal,
  });
}

/** 发起 DELETE 请求 */
export function apiDelete<T>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  return request<T>(path, {
    ...options.fetchOptions,
    method: 'DELETE',
    headers: options.headers,
    signal: options.signal,
  });
}
