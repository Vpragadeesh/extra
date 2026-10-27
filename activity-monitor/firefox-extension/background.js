const API = 'http://127.0.0.1:8765/api/events';
const TOKEN_API = 'http://127.0.0.1:8765/api/token';
const TICK_MS = 15000;

let currentTabId = null;
let currentWindowId = null;
let currentStartedAt = Date.now();
let currentTab = null;
let browserFocused = true;
let idleState = 'active';
let paused = false;
let excludedDomains = [];
let apiToken = null;
let timer = null;

async function loadSettings() {
  const storage = await browser.storage.local.get({ paused: false, excludedDomains: [] });
  paused = storage.paused;
  excludedDomains = storage.excludedDomains || [];
}

async function saveSettings() {
  await browser.storage.local.set({ paused, excludedDomains });
}

async function fetchApiToken() {
  try {
    const response = await fetch(TOKEN_API);
    if (response.ok) {
      const data = await response.json();
      apiToken = data.token;
      return true;
    }
  } catch (_) {
    // Backend may be stopped.
  }
  return false;
}

async function postEvent(payload) {
  const headers = { 'Content-Type': 'application/json', 'X-API-Token': apiToken || '' };
  let response = await fetch(API, { method: 'POST', headers, body: JSON.stringify(payload) });
  if (response.status === 401) {
    apiToken = null;
    if (await fetchApiToken()) {
      headers['X-API-Token'] = apiToken;
      response = await fetch(API, { method: 'POST', headers, body: JSON.stringify(payload) });
    }
  }
  return response;
}

// force=true is used when closing an interval (tab switch, idle, pause), so the
// time that was already active is not lost at the boundary.
async function send(payload, force = false) {
  if (!force && (paused || idleState !== 'active' || !browserFocused)) return;
  if (!apiToken && !(await fetchApiToken())) return;
  try {
    if (payload.url) {
      payload.url = stripUrl(payload.url);
      payload.domain = domainOf(payload.url);
    }
    await postEvent(payload);
  } catch (_) {
    // Backend may be stopped; never interrupt browsing.
  }
}

function domainOf(url) {
  try { return new URL(url).hostname; } catch { return null; }
}

function isExcluded(url) {
  const host = domainOf(url);
  if (!host) return true;
  if (host === 'localhost' || host === '127.0.0.1' || host === '::1' || host.endsWith('.local')) return true;
  return excludedDomains.some((d) => host === d || host.endsWith('.' + d));
}

function stripUrl(url) {
  try {
    const parsed = new URL(url);
    parsed.search = '';
    parsed.hash = '';
    return parsed.toString();
  } catch {
    return url;
  }
}

async function flushSession() {
  if (!currentTab || !currentStartedAt) return;
  const startedAt = currentStartedAt;
  const seconds = Math.max(0, (Date.now() - startedAt) / 1000);
  if (!isExcluded(currentTab.url)) {
    await send({
      source: 'firefox',
      event_type: 'tab_session',
      app: 'Firefox',
      title: currentTab.title || '',
      url: stripUrl(currentTab.url),
      duration_seconds: seconds,
      ts: new Date(startedAt).toISOString(),
      details: { tab_id: currentTabId }
    }, true);
  }
  currentTab = null;
  currentStartedAt = Date.now();
}

async function getActiveTab(win) {
  if (!win || win.id === undefined || win.id === browser.windows.WINDOW_ID_NONE) return null;
  const tabs = await browser.tabs.query({ active: true, windowId: win.id });
  return tabs[0] || null;
}

async function refreshActiveTab() {
  const win = await browser.windows.getLastFocused().catch(() => null);
  if (win) browserFocused = Boolean(win.focused);
  const tab = await getActiveTab(win).catch(() => null);
  const tabId = tab ? tab.id : null;
  const windowId = win ? win.id : null;

  if (!browserFocused) {
    await flushSession();
    currentTabId = tabId;
    currentWindowId = windowId;
    currentTab = null;
    return;
  }

  if (tabId === currentTabId && windowId === currentWindowId) {
    // Focus can return to the same tab after an unfocus/idle flush; restart
    // the interval instead of counting the away time as active.
    if (!currentTab) {
      currentTab = tab;
      currentStartedAt = Date.now();
    } else {
      currentTab = tab || currentTab;
    }
    return;
  }

  await flushSession();
  currentTabId = tabId;
  currentWindowId = windowId;
  currentTab = tab;
  currentStartedAt = Date.now();

  if (tab && !isExcluded(tab.url)) {
    await send({
      source: 'firefox',
      event_type: 'tab_focus',
      app: 'Firefox',
      title: tab.title || '',
      url: stripUrl(tab.url),
      ts: new Date().toISOString(),
      details: { tab_id: tab.id }
    });
  }
}

// Initial settings/token load, then start tracking.
loadSettings().then(async () => {
  await fetchApiToken();
  browser.storage.onChanged.addListener((changes) => {
    if (changes.paused) {
      const next = Boolean(changes.paused.newValue);
      if (next) {
        flushSession().then(() => { paused = true; });
      } else {
        paused = false;
        refreshActiveTab();
      }
    }
    if (changes.excludedDomains) {
      excludedDomains = changes.excludedDomains.newValue || [];
      flushSession();
    }
  });
  await refreshActiveTab();
  timer = setInterval(tick, TICK_MS);
});

browser.tabs.onActivated.addListener(() => { refreshActiveTab(); });

browser.tabs.onUpdated.addListener((id, change, tab) => {
  if (id !== currentTabId) return;
  if (!(change.url || change.title || change.status === 'complete')) return;
  const urlChanged = Boolean(change.url && currentTab && change.url !== currentTab.url);
  if (urlChanged) {
    flushSession().then(() => {
      currentTab = tab;
      currentStartedAt = Date.now();
      if (!isExcluded(tab.url)) {
        send({
          source: 'firefox',
          event_type: 'tab_focus',
          app: 'Firefox',
          title: tab.title || '',
          url: stripUrl(tab.url),
          ts: new Date().toISOString(),
          details: { tab_id: tab.id }
        });
      }
    });
  } else {
    currentTab = tab;
  }
});

browser.windows.onFocusChanged.addListener((id) => {
  const focused = id !== browser.windows.WINDOW_ID_NONE;
  if (!focused) {
    flushSession().then(() => { browserFocused = false; });
  } else {
    browserFocused = true;
    refreshActiveTab();
  }
});

browser.idle.setDetectionInterval(60);
browser.idle.onStateChanged.addListener((state) => {
  if (state !== 'active') {
    flushSession().then(() => { idleState = state; });
  } else {
    idleState = state;
    refreshActiveTab();
  }
});

browser.runtime.onMessage.addListener((message) => {
  if (message && message.type === 'setPaused') {
    const next = Boolean(message.paused);
    if (next) {
      flushSession().then(() => { paused = true; saveSettings(); });
    } else {
      paused = false;
      saveSettings();
      refreshActiveTab();
    }
    return Promise.resolve({ ok: true, paused: next });
  }
  return undefined;
});

async function tick() {
  if (!currentTab || isExcluded(currentTab.url)) return;
  const startedAt = currentStartedAt;
  const seconds = Math.max(0, (Date.now() - startedAt) / 1000);
  await send({
    source: 'firefox',
    event_type: 'tab_session',
    app: 'Firefox',
    title: currentTab.title || '',
    url: stripUrl(currentTab.url),
    duration_seconds: seconds,
    ts: new Date(startedAt).toISOString(),
    details: { tab_id: currentTabId, sample: true }
  });
  currentStartedAt = Date.now();
}
