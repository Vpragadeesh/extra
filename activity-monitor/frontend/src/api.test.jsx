import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, BASE, loadDashboard, pauseTracking } from './api';

const jsonResponse = (data, status = 200) =>
  Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(data),
  });

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn((url) => jsonResponse({ url })));
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('loadDashboard', () => {
  it('requests health, summary, events, and metrics with date + token', async () => {
    await loadDashboard({ day: '2026-10-26', token: 'sekrit' });
    const calls = window.fetch.mock.calls;
    expect(calls).toHaveLength(4);
    const urls = calls.map((c) => c[0]);
    expect(urls[0]).toBe(`${BASE}/health`);
    expect(urls[1]).toBe(`${BASE}/summary?date=2026-10-26`);
    expect(urls[2]).toBe(`${BASE}/events?limit=80&date=2026-10-26`);
    expect(urls[3]).toBe(`${BASE}/metrics?limit=500&date=2026-10-26`);
    for (const call of calls) {
      expect(call[1].headers['X-API-Token']).toBe('sekrit');
      expect(call[1].method).toBe('GET');
    }
  });

  it('omits the token header for token-less requests', async () => {
    await loadDashboard({ day: '2026-10-26' });
    for (const call of window.fetch.mock.calls) {
      expect(call[1].headers['X-API-Token']).toBeUndefined();
    }
  });

  it('never emits date=undefined when no day is given', async () => {
    await loadDashboard({});
    const urls = window.fetch.mock.calls.map((call) => call[0]);
    for (const url of urls) {
      expect(url).not.toContain('undefined');
    }
    expect(urls[1]).toBe(`${BASE}/summary`);
    expect(urls[2]).toBe(`${BASE}/events?limit=80`);
    expect(urls[3]).toBe(`${BASE}/metrics?limit=500`);
  });
});

describe('error handling', () => {
  it('throws ApiError with an HTTP status', async () => {
    vi.stubGlobal('fetch', vi.fn(() => jsonResponse({}, 401)));
    await expect(pauseTracking('tok', true)).rejects.toMatchObject({
      name: 'ApiError',
      status: 401,
    });
  });

  it('surfaces the API detail message when present', async () => {
    vi.stubGlobal('fetch', vi.fn(() => jsonResponse({ detail: 'Invalid token' }, 401)));
    const err = await loadDashboard({ day: '2026-10-26', token: 'bad' }).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.message).toBe('Invalid token');
  });

  it('rejects when fetch itself fails (backend offline)', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new TypeError('Failed to fetch'))));
    await expect(loadDashboard({ day: '2026-10-26' })).rejects.toBeInstanceOf(TypeError);
  });
});
