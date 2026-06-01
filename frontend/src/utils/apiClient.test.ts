import { afterAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { API_BASE_URL } from '../constants';
import { ApiError, apiGet, apiPost, extractErrorMessage } from './apiClient';

const originalFetch = global.fetch;

function buildExpectedUrl(path: string): string {
  const base = API_BASE_URL.replace(/\/+$/, '');
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  return `${base}${normalizedPath}`;
}

function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  const headers = new Headers(init.headers);
  if (!headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  return new Response(JSON.stringify(body), { ...init, headers });
}

describe('apiClient', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    global.fetch = fetchMock as typeof fetch;
  });

  afterAll(() => {
    global.fetch = originalFetch;
  });

  describe('ApiError', () => {
    it('保存 status、code 和 message', () => {
      const error = new ApiError('请求失败', { status: 400, code: 'bad_request' });

      expect(error).toBeInstanceOf(Error);
      expect(error).toBeInstanceOf(ApiError);
      expect(error.name).toBe('ApiError');
      expect(error.message).toBe('请求失败');
      expect(error.status).toBe(400);
      expect(error.code).toBe('bad_request');
    });

    it('无参数时 status 默认 0', () => {
      const error = new ApiError('网络错误');
      expect(error.status).toBe(0);
      expect(error.code).toBeUndefined();
    });
  });

  describe('apiGet', () => {
    it('成功时返回 JSON 并正确传递 headers/fetchOptions', async () => {
      fetchMock.mockResolvedValue(jsonResponse({ ok: true }, { status: 200 }));
      const controller = new AbortController();

      const result = await apiGet<{ ok: boolean }>('/api/health', {
        signal: controller.signal,
        headers: { 'X-Trace-ID': 'trace-1' },
        fetchOptions: { cache: 'no-store' },
      });

      expect(result).toEqual({ ok: true });
      expect(fetchMock).toHaveBeenCalledTimes(1);

      const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
      expect(url).toBe(buildExpectedUrl('/api/health'));
      expect(init.method).toBe('GET');
      expect(init.signal).toBe(controller.signal);
      expect(init.cache).toBe('no-store');
      expect(new Headers(init.headers).get('X-Trace-ID')).toBe('trace-1');
    });

    it('失败时解析新版 error envelope', async () => {
      fetchMock.mockResolvedValue(
        jsonResponse(
          { error: { code: 'invalid_query', message: '参数错误', request_id: 'req-123' } },
          { status: 400 },
        ),
      );

      await expect(apiGet('/api/health')).rejects.toMatchObject({
        name: 'ApiError',
        status: 400,
        code: 'invalid_query',
        message: '参数错误',
      });
    });

    it('失败时兼容旧版 error 字符串', async () => {
      fetchMock.mockResolvedValue(
        jsonResponse({ error: '旧版错误消息' }, { status: 500 }),
      );

      await expect(apiGet('/api/health')).rejects.toMatchObject({
        name: 'ApiError',
        status: 500,
        message: '旧版错误消息',
      });
    });

    it('网络错误时抛出 network_error ApiError', async () => {
      fetchMock.mockRejectedValue(new Error('network down'));

      await expect(apiGet('/api/health')).rejects.toMatchObject({
        name: 'ApiError',
        status: 0,
        code: 'network_error',
        message: 'network down',
      });
    });
  });

  describe('apiPost', () => {
    it('成功时发送 JSON body', async () => {
      fetchMock.mockResolvedValue(jsonResponse({ id: 'job-1' }, { status: 200 }));

      const result = await apiPost<{ id: string }>('/api/onboarding/test', { provider: 'openai' });

      expect(result).toEqual({ id: 'job-1' });

      const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
      expect(url).toBe(buildExpectedUrl('/api/onboarding/test'));
      expect(init.method).toBe('POST');
      expect(init.body).toBe(JSON.stringify({ provider: 'openai' }));
      expect(new Headers(init.headers).get('Content-Type')).toBe('application/json');
    });

    it('支持额外 headers 和 fetchOptions', async () => {
      fetchMock.mockResolvedValue(jsonResponse({ ok: true }, { status: 200 }));

      await apiPost('/api/test', {}, {
        headers: { 'X-Request-ID': 'req-1' },
        fetchOptions: { cache: 'no-store' },
      });

      const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
      expect(init.cache).toBe('no-store');
      expect(new Headers(init.headers).get('X-Request-ID')).toBe('req-1');
    });

    it('失败时解析错误', async () => {
      fetchMock.mockResolvedValue(jsonResponse({ error: '提交失败' }, { status: 422 }));

      await expect(apiPost('/api/onboarding/test', {})).rejects.toMatchObject({
        name: 'ApiError',
        status: 422,
        message: '提交失败',
      });
    });
  });

  describe('边界情况', () => {
    it('空响应体（204）成功返回 undefined', async () => {
      fetchMock.mockResolvedValue(new Response('', { status: 200 }));
      const result = await apiGet('/api/health');
      expect(result).toBeUndefined();
    });

    it('错误响应体为空时回退到默认消息', async () => {
      fetchMock.mockResolvedValue(new Response('', { status: 500 }));
      await expect(apiGet('/api/health')).rejects.toMatchObject({
        name: 'ApiError',
        status: 500,
        message: '请求失败 (500)',
      });
    });

    it('apiPost 不覆盖调用方自定义 Content-Type', async () => {
      fetchMock.mockResolvedValue(jsonResponse({ ok: true }, { status: 200 }));
      await apiPost('/api/upload', {}, {
        headers: { 'Content-Type': 'text/plain' },
      });
      const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
      expect(new Headers(init.headers).get('Content-Type')).toBe('text/plain');
    });
  });

  describe('extractErrorMessage', () => {
    it('解析新版 error envelope', () => {
      const text = '{"error":{"code":"bad_request","message":"新版错误"}}';
      expect(extractErrorMessage(text, 'fallback')).toBe('新版错误');
    });

    it('解析旧版 error 字符串', () => {
      expect(extractErrorMessage('{"error":"旧版错误"}', 'fallback')).toBe('旧版错误');
    });

    it('简短非 JSON 文本时回退到原始文本', () => {
      expect(extractErrorMessage('gateway timeout', 'fallback')).toBe('gateway timeout');
    });

    it('HTML 网关错误页不暴露原始 body，回退到 fallback', () => {
      const html = '<!DOCTYPE html><html><head><title>502 Bad gateway</title></head><body>...</body></html>';
      expect(extractErrorMessage(html, 'fallback')).toBe('fallback');
    });

    it('超长非 JSON body 回退到 fallback', () => {
      expect(extractErrorMessage('x'.repeat(500), 'fallback')).toBe('fallback');
    });

    it('空文本时使用 fallback', () => {
      expect(extractErrorMessage('', 'fallback')).toBe('fallback');
    });
  });

  describe('网关瞬断重试', () => {
    it('GET 遇 502 静默重试一次并在恢复后成功', async () => {
      fetchMock
        .mockResolvedValueOnce(new Response('<html>502 Bad gateway</html>', { status: 502 }))
        .mockResolvedValueOnce(jsonResponse({ ok: true }, { status: 200 }));
      const result = await apiGet<{ ok: boolean }>('/api/admin/monitors');
      expect(result).toEqual({ ok: true });
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });

    it('GET 持续 502 时重试耗尽，抛出干净的状态文案（不含 HTML）', async () => {
      // 每次调用返回全新 Response（真实 fetch 行为；body 只能读一次）
      fetchMock.mockImplementation(() => new Response('<html>502 Bad gateway</html>', { status: 502 }));
      await expect(apiGet('/api/admin/monitors')).rejects.toMatchObject({
        name: 'ApiError',
        status: 502,
        message: '服务暂时不可用（502），请稍后重试',
      });
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });

    it('POST（非幂等）遇 502 不重试', async () => {
      fetchMock.mockResolvedValue(new Response('<html>502</html>', { status: 502 }));
      await expect(apiPost('/api/admin/monitors', {})).rejects.toMatchObject({
        status: 502,
        message: '服务暂时不可用（502），请稍后重试',
      });
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    it('GET 遇 500（应用错误）不重试', async () => {
      fetchMock.mockResolvedValue(jsonResponse({ error: '内部错误' }, { status: 500 }));
      await expect(apiGet('/api/status')).rejects.toMatchObject({ status: 500 });
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });
  });

  describe('AbortSignal', () => {
    it('取消时透传 AbortError（不包装为 ApiError）', async () => {
      fetchMock.mockImplementation((_input: RequestInfo | URL, init?: RequestInit) => {
        return new Promise<Response>((_resolve, reject) => {
          const signal = init?.signal;
          if (signal) {
            const abortError = new Error('The operation was aborted.');
            abortError.name = 'AbortError';
            signal.addEventListener('abort', () => reject(abortError), { once: true });
          }
        });
      });

      const controller = new AbortController();
      const promise = apiGet('/api/slow', { signal: controller.signal });
      controller.abort();

      await expect(promise).rejects.toMatchObject({ name: 'AbortError' });
    });
  });
});
