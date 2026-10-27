// Single fetch client for the local API. Every dashboard network call goes
// through here so query shapes, headers, and error text stay consistent.

export const BASE = 'http://127.0.0.1:8765/api';

export class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function request(path, { token, method = 'GET', body } = {}) {
  const headers = {};
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (token) headers['X-API-Token'] = token;
  const response = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!response.ok) {
    let message = `API ${path} returned ${response.status}`;
    try {
      const data = await response.json();
      if (data?.detail) message = typeof data.detail === 'string' ? data.detail : message;
    } catch { /* non-JSON error body */ }
    throw new ApiError(message, response.status);
  }
  if (response.status === 204) return null;
  return response.json();
}

export const getToken = () => request('/token');
export const getSettings = () => request('/settings');
export const saveSettings = (token, payload) => request('/settings', { method: 'POST', token, body: payload });

export const loadDashboard = async ({ day, token } = {}) => {
  const base = { token };
  // Never emit `date=undefined`: omit the parameter entirely when no day is
  // given (the backend then applies its own default of today).
  const dayQs = day ? `?date=${encodeURIComponent(day)}` : '';
  const dayAmp = day ? `&date=${encodeURIComponent(day)}` : '';
  const [health, summary, events, metrics] = await Promise.all([
    request('/health', base),
    request(`/summary${dayQs}`, base),
    request(`/events?limit=80${dayAmp}`, base),
    request(`/metrics?limit=500${dayAmp}`, base),
  ]);
  return { health, summary, events, metrics };
};

export const pauseTracking = (token, paused) => request('/control', { token, method: 'POST', body: { paused } });
export const deleteAllData = (token) => request('/control/delete-all', { token, method: 'POST' });
