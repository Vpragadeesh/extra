import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Activity, AppWindow, Clock3, Cpu, Database, Globe, Pause, Play,
  RefreshCw, ShieldCheck, Terminal, Wifi,
} from 'lucide-react';
import ErrorBanner from './components/ErrorBanner';
import { getToken, getSettings, saveSettings as apiSaveSettings, loadDashboard, pauseTracking, deleteAllData } from './api';
import { localDay } from './lib/format';
import OverviewView from './views/OverviewView';
import BrowserView from './views/BrowserView';
import TerminalView from './views/TerminalView';
import SystemView from './views/SystemView';
import PrivacyView, { validatePrivacyDraft } from './views/PrivacyView';

import '@fontsource/fira-sans/400.css';
import '@fontsource/fira-sans/500.css';
import '@fontsource/fira-sans/600.css';
import '@fontsource/fira-code/400.css';
import './index.css';

const VIEWS = [
  { id: 'overview', label: 'Overview', icon: Activity },
  { id: 'browser', label: 'Browser activity', icon: Globe },
  { id: 'terminal', label: 'Terminal', icon: Terminal },
  { id: 'system', label: 'System metrics', icon: Cpu },
  { id: 'privacy', label: 'Privacy settings', icon: ShieldCheck },
];

const VIEW_COPY = {
  overview: { title: 'Your activity, at a glance', subtitle: 'A private overview of how you use your Linux machine.' },
  browser: { title: 'Browser activity', subtitle: 'Firefox domains, visits, and recent tab sessions.' },
  terminal: { title: 'Terminal activity', subtitle: 'Commands recorded by the fish hook, limited to the first token.' },
  system: { title: 'System metrics', subtitle: 'CPU, memory, network, and optional GPU samples.' },
  privacy: { title: 'Privacy settings', subtitle: 'Control exclusions, retention, and what leaves memory.' },
};

const REFRESH_CHOICES = [
  { value: 'off', label: 'off', ms: null },
  { value: '5000', label: '5s', ms: 5000 },
  { value: '10000', label: '10s', ms: 10000 },
  { value: '30000', label: '30s', ms: 30000 },
  { value: '60000', label: '60s', ms: 60000 },
];

const REFRESH_KEY = 'am:refreshInterval';
const readRefreshChoice = () => {
  try {
    const stored = window.localStorage.getItem(REFRESH_KEY);
    return REFRESH_CHOICES.some((c) => c.value === stored) ? stored : '10000';
  } catch {
    return '10000';
  }
};

const prefersReducedMotion = () =>
  typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

export default function App() {
  const today = localDay();
  const [view, setView] = useState(() => {
    const hash = window.location.hash.replace('#', '');
    return VIEWS.some((item) => item.id === hash) ? hash : 'overview';
  });
  const [day, setDay] = useState(today);
  const [health, setHealth] = useState(null);
  const [summary, setSummary] = useState(null);
  const [events, setEvents] = useState([]);
  const [metrics, setMetrics] = useState([]);
  const [settings, setSettings] = useState(null);
  const [draft, setDraft] = useState(null);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);
  const [firstLoad, setFirstLoad] = useState(true);
  const [apiToken, setApiToken] = useState(null);
  const [showToken, setShowToken] = useState(false);
  const [tokenCopied, setTokenCopied] = useState(false);
  const [refreshChoice, setRefreshChoice] = useState(readRefreshChoice);

  const settingsLoaded = useRef(false);
  const refreshRun = useRef(0);
  const reduceMotion = useRef(prefersReducedMotion());

  const applySettings = useCallback((value) => {
    settingsLoaded.current = true;
    setSettings(value);
    setDraft({
      excluded_domains: (value.excluded_domains || []).join('\n'),
      excluded_apps: (value.excluded_apps || []).join('\n'),
      retention_days: value.retention_days ?? 30,
      strip_query_fragments: Boolean(value.strip_query_fragments),
      domain_storage_mode: value.domain_storage_mode || 'full',
      exclude_titles: Boolean(value.exclude_titles),
      store_cwd: value.store_cwd !== false,
      metrics_interval_seconds: value.metrics_interval_seconds ?? 5,
    });
  }, []);

  const loadSettings = useCallback(async () => {
    try {
      const res = await getSettings();
      if (res) applySettings(res);
    } catch { /* backend offline */ }
  }, [applySettings]);

  const refresh = useCallback(async () => {
    // Sequencing guard: when refreshes overlap (date change, interval tick
    // while one is in flight), only the newest run is allowed to write state.
    const run = ++refreshRun.current;
    setBusy(true);
    try {
      // Recover gracefully when the dashboard opens while the backend was
      // offline: re-fetch the token before the data requests.
      let token = apiToken;
      if (!token) {
        try {
          const data = await getToken();
          token = data.token;
          setApiToken(token);
        } catch { /* data fetch below reports the real error */ }
      }
      const result = await loadDashboard({ day: day || today, token });
      if (run !== refreshRun.current) return; // superseded by a newer refresh
      setHealth(result.health);
      setSummary(result.summary);
      setEvents(result.events);
      setMetrics(result.metrics);
      setError('');
      if (!settingsLoaded.current) loadSettings();
    } catch {
      if (run !== refreshRun.current) return; // a newer attempt owns the UI state
      setError('Backend unavailable. Start the Python service at 127.0.0.1:8765 (or the systemd user service) and refresh.');
    } finally {
      if (run === refreshRun.current) {
        setFirstLoad(false);
        setBusy(false);
      }
    }
  }, [apiToken, day, today, loadSettings]);

  useEffect(() => { refresh(); }, [refresh]); // refresh() identity changes with apiToken/day
  useEffect(() => {
    if (refreshChoice === 'off') return undefined;
    const id = setInterval(() => { refresh(); }, Number(refreshChoice));
    return () => clearInterval(id);
  }, [refresh, refreshChoice]);

  function selectView(id) {
    setView(id);
    if (window.location.hash !== `#${id}`) {
      window.history.replaceState(null, '', `#${id}`);
    }
  }

  useEffect(() => {
    const onHashChange = () => {
      const next = window.location.hash.replace('#', '');
      if (VIEWS.some((item) => item.id === next)) setView(next);
    };
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  // Normalize an unknown initial hash (#bogus) so URL and view agree.
  useEffect(() => {
    if (!VIEWS.some((item) => item.id === window.location.hash.replace('#', ''))) {
      window.history.replaceState(null, '', '#overview');
    }
  }, []);

  useEffect(() => {
    // Track the OS motion preference live instead of only at mount time.
    if (typeof window.matchMedia !== 'function') return undefined;
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)');
    const onChange = (event) => { reduceMotion.current = event.matches; };
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);

  useEffect(() => {
    window.scrollTo({ top: 0, behavior: reduceMotion.current ? 'auto' : 'smooth' });
  }, [view]);

  useEffect(() => {
    try { window.localStorage.setItem(REFRESH_KEY, refreshChoice); } catch { /* private mode */ }
  }, [refreshChoice]);

  async function togglePause() {
    if (!apiToken) { setError('API token not loaded yet. Please wait a moment.'); return; }
    try {
      await pauseTracking(apiToken, !health?.paused);
      await refresh();
    } catch {
      setError('Could not change tracking state. Check the backend and token.');
    }
  }

  async function copyToken() {
    if (!apiToken) return;
    try {
      await navigator.clipboard.writeText(apiToken);
      setTokenCopied(true);
      setTimeout(() => setTokenCopied(false), 2000);
    } catch {
      setNotice('Clipboard access was blocked. Select the token text manually.');
    }
  }

  const privacyErrors = useMemo(() => validatePrivacyDraft(draft), [draft]);

  async function handleSaveSettings() {
    if (!draft || !apiToken) { setError('API token not loaded yet. Please wait a moment.'); return; }
    if (Object.keys(privacyErrors).length) {
      setError('Fix the highlighted privacy fields before saving.');
      return;
    }
    const payload = {
      excluded_domains: draft.excluded_domains.split('\n').map((s) => s.trim()).filter(Boolean),
      excluded_apps: draft.excluded_apps.split('\n').map((s) => s.trim()).filter(Boolean),
      retention_days: Number(draft.retention_days),
      strip_query_fragments: draft.strip_query_fragments,
      domain_storage_mode: draft.domain_storage_mode,
      exclude_titles: draft.exclude_titles,
      store_cwd: draft.store_cwd,
      metrics_interval_seconds: Number(draft.metrics_interval_seconds),
    };
    try {
      const saved = await apiSaveSettings(apiToken, payload);
      applySettings(saved);
      setNotice('Privacy settings saved.');
      setError('');
      await refresh();
    } catch {
      setError('Could not save privacy settings. Check the backend and token.');
    }
  }

  async function handleDeleteAll() {
    if (!apiToken) { setError('API token not loaded yet. Please wait a moment.'); return; }
    // Confirmation is intentionally explicit: this wipes the entire local store.
    if (!window.confirm('Delete all locally stored activity events and metrics? This cannot be undone.')) return;
    try {
      await deleteAllData(apiToken);
      setNotice('All stored activity data was deleted.');
      await refresh();
    } catch {
      setError('Could not delete activity data. Check the backend and token.');
    }
  }

  const loading = firstLoad && !error;
  const currentView = VIEWS.find((item) => item.id === view) || VIEWS[0];
  const viewCopy = VIEW_COPY[view] || VIEW_COPY.overview;

  const collectors = health?.collectors || {};
  const collectorEntries = Object.entries(collectors);
  const collectorsOk = collectorEntries.filter(([, value]) => value.ok).length;
  const allCollectorsOk = collectorEntries.length > 0 && collectorsOk === collectorEntries.length;
  const statusTone = !health?.ok
    ? 'border-amber-300/15 bg-amber-300/[.07] text-amber-200'
    : allCollectorsOk
      ? 'border-emerald-300/15 bg-emerald-300/[.07] text-emerald-200'
      : 'border-sky-300/15 bg-sky-300/[.07] text-sky-200';
  const statusText = !health?.ok ? 'Waiting for backend' : allCollectorsOk ? 'All collectors healthy' : `Collectors ${collectorsOk}/${collectorEntries.length || 3} healthy`;
  const statusDetail = collectorEntries.map(([name, value]) => `${name}: ${value.ok ? 'ok' : value.last_error || 'starting'}`).join('\n');

  return (
    <div className="min-h-screen text-slate-100">
      <aside className="fixed inset-y-0 left-0 hidden w-64 border-r border-white/[.07] bg-[#080d19]/80 px-5 py-7 backdrop-blur-xl lg:block">
        <div className="flex items-center gap-3">
          <div className="rounded-xl bg-sky-400/15 p-2.5 text-sky-300"><Activity size={23} /></div>
          <div>
            <div className="font-semibold tracking-tight">Activity Monitor</div>
            <div className="text-xs text-slate-500">Linux · local analytics</div>
          </div>
        </div>
        <div className="mt-12 text-[10px] font-semibold uppercase tracking-[.2em] text-slate-600">Workspace</div>
        <nav aria-label="Workspace" className="mt-3 space-y-1">
          {VIEWS.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              type="button"
              onClick={() => selectView(id)}
              aria-current={view === id ? 'page' : undefined}
              className={`flex w-full cursor-pointer items-center gap-3 rounded-xl px-3 py-3 text-left text-sm transition duration-200 ${
                view === id
                  ? 'border border-sky-300/10 bg-sky-300/[.08] text-sky-100'
                  : 'border border-transparent text-slate-500 hover:bg-white/[.035] hover:text-slate-300'
              }`}
            >
              <Icon size={17} /> {label}
            </button>
          ))}
        </nav>
        <div className="absolute bottom-6 left-5 right-5 rounded-2xl border border-white/[.08] bg-white/[.035] p-4">
          <div className="flex items-center gap-2 text-sm"><ShieldCheck size={16} className="text-emerald-300" /> Local-first privacy</div>
          <p className="mt-2 text-xs leading-5 text-slate-500">Your activity database stays on this machine. No cloud account or telemetry.</p>
        </div>
      </aside>

      <main className="mx-auto max-w-[1600px] px-4 py-7 sm:px-8 lg:ml-64 lg:px-10 lg:py-9">
        <header className="flex flex-col justify-between gap-5 xl:flex-row xl:items-center">
          <div>
            <div className="text-sm text-slate-500">Workspace / {currentView.label}</div>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight sm:text-3xl lg:text-4xl">{viewCopy.title}<span className="text-sky-300">.</span></h1>
            <p className="mt-2 text-sm text-slate-400">{viewCopy.subtitle}</p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <label className="flex items-center gap-2 rounded-xl border border-white/10 bg-white/[.05] px-3 py-2 text-xs text-slate-300">
              <Clock3 size={14} />
              <span className="sr-only">Summary date</span>
              <input type="date" value={day} max={today} onChange={(e) => setDay(e.target.value || today)} className="cursor-pointer bg-transparent text-xs text-slate-200 outline-none [color-scheme:dark]" />
            </label>
            <span className={`flex items-center gap-2 rounded-full border px-3 py-2 text-xs ${statusTone}`} title={statusDetail || statusText}>
              <span className={`h-1.5 w-1.5 rounded-full ${!health?.ok ? 'bg-amber-300' : allCollectorsOk ? 'bg-emerald-300' : 'bg-sky-300'}`} />
              {statusText}
            </span>
            <label className="flex items-center gap-2 rounded-xl border border-white/10 bg-white/[.05] px-3 py-2 text-xs text-slate-300" aria-label="Auto-refresh interval">
              <RefreshCw size={13} className={busy ? 'motion-safe:animate-spin' : 'opacity-60'} />
              <select
                value={refreshChoice}
                onChange={(e) => setRefreshChoice(e.target.value)}
                className="cursor-pointer bg-transparent text-xs text-slate-200 outline-none [color-scheme:dark]"
              >
                {REFRESH_CHOICES.map((c) => <option key={c.value} value={c.value}>refresh {c.label}</option>)}
              </select>
            </label>
            <button onClick={refresh} className="cursor-pointer rounded-xl border border-white/10 bg-white/[.05] p-2.5 text-slate-300 transition duration-200 hover:bg-white/10" title="Refresh now" aria-label="Refresh data">
              <RefreshCw size={16} />
            </button>
            <button onClick={togglePause} disabled={!health?.ok || !apiToken} className="flex cursor-pointer items-center gap-2 rounded-xl bg-sky-300 px-4 py-2.5 text-sm font-semibold text-slate-950 transition duration-200 hover:bg-sky-200 disabled:cursor-not-allowed disabled:opacity-40">
              {health?.paused ? <Play size={16} /> : <Pause size={16} />}
              {health?.paused ? 'Resume' : 'Pause'}
            </button>
          </div>
        </header>

        <nav className="mt-5 flex gap-2 overflow-x-auto pb-1 lg:hidden" aria-label="Workspace views (mobile)">
          {VIEWS.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              type="button"
              onClick={() => selectView(id)}
              aria-current={view === id ? 'page' : undefined}
              className={`flex shrink-0 cursor-pointer items-center gap-2 rounded-xl px-3 py-2 text-xs transition duration-200 ${
                view === id
                  ? 'border border-sky-300/10 bg-sky-300/[.08] text-sky-100'
                  : 'border border-white/[.07] bg-white/[.03] text-slate-400'
              }`}
            >
              <Icon size={14} /> {label}
            </button>
          ))}
        </nav>

        {error && <ErrorBanner title={error} troubleshooting />}
        {notice && (
          <div role="status" className="mt-6 rounded-xl border border-emerald-300/20 bg-emerald-300/[.06] px-4 py-3 text-sm text-emerald-100">
            {notice}
          </div>
        )}

        {view === 'overview' && (
          <OverviewView summary={summary} events={events} metrics={metrics} settings={settings} day={day} today={today} loading={loading} />
        )}
        {view === 'browser' && <BrowserView summary={summary} events={events} loading={loading} />}
        {view === 'terminal' && <TerminalView events={events} loading={loading} />}
        {view === 'system' && <SystemView metrics={metrics} settings={settings} loading={loading} />}
        {view === 'privacy' && (
          <PrivacyView
            settings={settings}
            draft={draft}
            setDraft={setDraft}
            errors={privacyErrors}
            showToken={showToken}
            setShowToken={setShowToken}
            tokenCopied={tokenCopied}
            copyToken={copyToken}
            saveSettings={handleSaveSettings}
            deleteAll={handleDeleteAll}
            apiToken={apiToken}
          />
        )}

        <footer className="mt-8 flex flex-col gap-2 border-t border-white/[.07] pt-5 text-xs text-slate-600 sm:flex-row sm:items-center sm:justify-between">
          <span className="flex items-center gap-2"><ShieldCheck size={14} className="text-emerald-400" /> Local-only storage · SQLite · No cloud sync</span>
          <span className="flex items-center gap-2"><Wifi size={13} /> API: 127.0.0.1:8765 <span className="text-slate-700">·</span> <Database size={13} /> {summary?.event_count ?? 0} events on {day}</span>
        </footer>
      </main>
    </div>
  );
}
