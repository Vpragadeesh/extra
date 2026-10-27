import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import App from './App';

const SETTINGS = {
  excluded_domains: [],
  excluded_apps: [],
  retention_days: 30,
  strip_query_fragments: true,
  domain_storage_mode: 'full',
  exclude_titles: false,
  store_cwd: true,
  metrics_interval_seconds: 5,
};

const EMPTY_DASHBOARD = {
  health: { ok: true, paused: false, collectors: { hyprland: { ok: true }, fish: { ok: true }, metrics: { ok: true } } },
  summary: { apps: [], sites: [], tracked_seconds: 0, site_count: 0, event_count: 0 },
  events: [],
  metrics: [],
};

function stubFetch(overrides = {}) {
  const impl = (url) => {
    if (typeof url !== 'string') return Promise.reject(new TypeError('Failed to fetch'));
    if (overrides.offline) return Promise.reject(new TypeError('Failed to fetch'));
    if (url.includes('/api/token')) return Promise.resolve(json({ token: 'tok-123' }));
    if (url.includes('/api/settings') && overrides.failSettings) return Promise.reject(new TypeError('Failed to fetch'));
    if (url.includes('/api/settings')) return Promise.resolve(json(SETTINGS));
    if (url.includes('/api/health')) return Promise.resolve(json(overrides.health || EMPTY_DASHBOARD.health));
    if (url.includes('/api/summary')) return Promise.resolve(json(EMPTY_DASHBOARD.summary));
    if (url.includes('/api/events')) return Promise.resolve(json([]));
    if (url.includes('/api/metrics')) return Promise.resolve(json([]));
    return Promise.reject(new TypeError('Unexpected fetch: ' + url));
  };
  const fetchMock = vi.fn(impl);
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const json = (data) => Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(data) });

beforeEach(() => {
  window.localStorage.clear();
  window.scrollTo = vi.fn();
  window.location.hash = '#overview';
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('App offline state', () => {
  it('shows an announced error when the backend is unreachable', async () => {
    stubFetch({ offline: true });
    render(<App />);
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Backend unavailable');
    expect(await screen.findByText(/systemctl --user status activity-monitor/)).toBeTruthy();
  });
});

describe('Privacy settings per-field errors', () => {
  it('flags an invalid excluded-domains line next to the field and blocks saving', async () => {
    stubFetch({});
    render(<App />);

    // Switch to Privacy view; the form appears once settings load.
    const privacyButtons = screen.getAllByRole('button', { name: 'Privacy settings' });
    fireEvent.click(privacyButtons[0]);

    const domainsField = await screen.findByLabelText(/Excluded domains/i);
    fireEvent.change(domainsField, { target: { value: 'bad domain,here\nok.example' } });

    expect(await screen.findByText(/Use one domain per line/i)).toHaveAttribute('id', 'excluded-domains-error');
    expect(domainsField).toHaveAttribute('aria-describedby', 'excluded-domains-error');
    expect(screen.getByLabelText(/Excluded domains/i)).toHaveAttribute('aria-invalid', 'true');
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled();
  });

  it('validates retention bounds inline', async () => {
    stubFetch({});
    render(<App />);
    const privacyButtons = screen.getAllByRole('button', { name: 'Privacy settings' });
    await waitFor(() => privacyButtons.length > 0);
    fireEvent.click(privacyButtons[0]);

    const retention = await screen.findByLabelText(/Retention \(days\)/i);
    fireEvent.change(retention, { target: { value: '99999' } });
    expect(await screen.findByText(/Whole number between 1 and 3650/i)).toBeInTheDocument();
    expect(retention).toHaveAttribute('aria-invalid', 'true');
  });
});
