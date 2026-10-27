# Local Activity Monitor

Local-first activity tracker for Linux, built around Hyprland, fish, and Firefox, with a FastAPI + SQLite backend and a React/Vite/Tailwind dashboard.

## What it tracks

- Hyprland active-window app class/title and approximate foreground duration, paused when systemd-logind reports the session idle or locked.
- Firefox active-tab URL/title and active time while the browser window is focused and not idle (extension required).
- fish command executable (first token only) and working directory (can be disabled).
- CPU, memory, network byte deltas, and optional NVIDIA GPU metrics, sampled every 2–300 seconds (default 5).

## Privacy model

- Data stays in `~/.local/share/activity-monitor/activity.db` by default.
- The API binds to `127.0.0.1:8765` only; it is not designed for LAN exposure.
- No telemetry, cloud sync, DOM inspection, cookie access, keystrokes, screenshots, or form-content collection.
- Window titles and URLs can still contain sensitive data. Use exclusions, domain-only mode, title omission, cwd omission, and pause.
- Query strings/fragments are stripped by default.
- Default retention is 30 days with automatic daily cleanup, and a delete-all action is available.
- API write endpoints require a per-install random token. Browser origins are restricted, and non-loopback `Host` headers are rejected (DNS-rebinding protection).

## Requirements

- Arch Linux with a Hyprland session (the Hyprland collector is a no-op on other compositors; see [Known limitations](#known-limitations)).
- Python 3.11+ (tested with 3.14).
- Node.js 18+ / npm (tested with Node 26).
- fish 3.6+ (tested with fish 4.9).
- Firefox 109+ for the extension.
- `curl` for the fish hook.
- Optional: `nvidia-utils` for GPU metrics.

Install the base packages:

```bash
sudo pacman -S --needed python nodejs npm curl
```

## Backend setup (uv)

```bash
cd backend
uv venv .venv
uv pip install --python .venv/bin/python -r requirements.txt
.venv/bin/python -m activity_monitor
```

The API is then available at `http://127.0.0.1:8765` (docs at `/docs`).

For development dependencies (tests):

```bash
uv pip install --python .venv/bin/python -r requirements-dev.txt
```

## Frontend setup

In a second terminal:

```bash
cd frontend
pnpm install   # pnpm manages frontend deps; npm works but the lockfile of record is pnpm's
pnpm run dev
```

Open the Vite URL (normally `http://localhost:5173`). The backend must be running for data; the dashboard shows an announced API-offline state otherwise. The sidebar switches between Overview, Browser activity, Terminal, System metrics, and Privacy settings. The date picker selects the local calendar day used by the summary, event lists, and metric charts, and the header select picks the auto-refresh interval (off/5s/10s/30s/60s, remembered in `localStorage`).

The dashboard is structured rather than one file: `src/api.js` (the only network layer, used by tests), `src/lib/format.js` (pure formatters), `src/components/*` (MetricCard, Timeline, SystemCharts, skeletons, error banner), `src/views/*` (one file per sidebar view), and `src/App.jsx` (state, routing, refresh loop). Fonts (Fira Sans/Fira Code) are self-hosted via `@fontsource` — no font requests leave the machine. Keyboard focus is visibly highlighted, errors/announcements use `role="alert"`/`role="status"`, the privacy form validates per field with `aria-describedby`, and all motion stops under `prefers-reduced-motion: reduce`. Verified without horizontal overflow at 375/768/1024/1440 widths.

Production build:

```bash
pnpm run build
```

## Tests

Run all repeatable checks (backend tests, frontend tests, frontend build, fish syntax, extension syntax, manifest JSON):

```bash
./scripts/test.sh
```

Backend tests alone:

```bash
cd backend
.venv/bin/python -m pytest -q
```

## Install the Firefox extension

1. Open `about:debugging#/runtime/this-firefox` in Firefox.
2. Click **Load Temporary Add-on…** and select `firefox-extension/manifest.json`.
3. Temporary add-ons must be reloaded after Firefox restarts. For a persistent install, package/sign the extension.

The extension uses **Manifest V3** (Firefox event-page background). MV2 `background.scripts` no longer executes on current Firefox releases (verified on Firefox 158: an MV2 install loads but its background never runs and reports nothing).

Permissions requested and why:

- `tabs` — read the active tab's URL/title.
- `storage` — save the local pause toggle and domain exclusions.
- `idle` — stop counting time when the browser/system is idle.
- `host_permissions: http://127.0.0.1:8765/*` — post metadata to the local API only.

Runtime verification (2026-10-27, Firefox 158.0.1 via `web-ext run --args=-headless` against the live API): tab navigation produced a `tab_focus` event for `https://example.com/` with domain normalization, and 15-second `tab_session` samples landed with correct durations. The backend must answer the extension's CORS preflight (OPTIONS) for `moz-extension://` origins — done via `allow_origin_regex` in the API — or tab events are silently dropped.

The extension fetches the local API token on startup and refreshes it if a request returns 401. It never reads page contents, forms, cookies, or passwords.

Extension options (via `about:addons` → Local Activity Monitor → Preferences) let you pause browser tracking and maintain domain exclusions.

## fish hook

Source the hook from `~/.config/fish/config.fish`:

```fish
source /path/to/activity-monitor/fish/activity-monitor.fish
```

Configuration:

```fish
# Optional: disable tracking entirely
# set -gx ACTIVITY_MONITOR_DISABLE 1

# Required for writes: the API rejects events without this token.
# Copy it from the dashboard's Privacy and retention panel, or:
#   curl -s http://127.0.0.1:8765/api/token
set -gx ACTIVITY_MONITOR_TOKEN your-token-here
```

The hook submits only the first command token and `$PWD`, runs `curl` in the background with a short timeout, and never blocks or breaks interactive shell use when the backend is down. The backend drops `cwd` when `store_cwd` is disabled. Without `ACTIVITY_MONITOR_TOKEN`, the backend returns 401 and the event is dropped.

## Auto-start with systemd

The unit in `systemd/activity-monitor.service` assumes the project is at `~/extra/activity-monitor`. If yours is elsewhere, edit `WorkingDirectory` and `ExecStart` before installing.

```bash
mkdir -p ~/.config/systemd/user
cp systemd/activity-monitor.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now activity-monitor.service
systemctl --user status activity-monitor.service
journalctl --user -u activity-monitor.service -f
```

Disable/uninstall:

```bash
systemctl --user disable --now activity-monitor.service
rm ~/.config/systemd/user/activity-monitor.service
# remove the fish `source` line and the Firefox temporary add-on
# optionally delete the database:
rm -rf ~/.local/share/activity-monitor
```

## Data, retention, and deletion

- Database: `~/.local/share/activity-monitor/activity.db` (override with `ACTIVITY_MONITOR_DB`).
- Timestamps are stored in UTC. The dashboard renders them in local time.
- `net_sent_bytes` / `net_recv_bytes` store **per-sample deltas** (not cumulative counters), with counter-reset handling.
- Retention defaults to 30 days; cleanup runs at startup and every 24 hours.
- Dashboard → **Privacy and retention** can update exclusions, retention, domain-only mode, title omission, cwd storage, metrics sampling interval, and query stripping.
- The same controls are available through `GET/POST /api/settings`; deletion uses `POST /api/control/delete-all` with confirmation in the UI.
- `store_cwd` controls whether the fish working directory is persisted (default `true`). The hook still sends it, but the backend drops it before storage when disabled.
- `metrics_interval_seconds` controls system-metrics sampling (range 2–300, default 5). A change takes effect after the current wait completes.

### SQLite schema (v1)

`events`:

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK | Auto-increment event ID |
| `ts` | TEXT | ISO-8601 UTC timestamp |
| `source` | TEXT | `hyprland`, `firefox`, or `fish` |
| `event_type` | TEXT | `window_focus`, `window_session`, `tab_focus`, `tab_session`, or `command` |
| `app` | TEXT | Normalized app class / `Firefox` |
| `title` | TEXT | Window/page title, subject to `exclude_titles` |
| `url` | TEXT | Firefox URL, stripped/omitted by privacy settings |
| `domain` | TEXT | Normalized hostname |
| `cwd` | TEXT | fish working directory; omitted when `store_cwd` is false |
| `command` | TEXT | First command token only |
| `duration_seconds` | REAL | Estimated interval duration for `*_session` events |
| `details_json` | TEXT | Small allowlisted metadata (window address/workspace, tab ID, sample marker) |

`metrics`:

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK | Auto-increment sample ID |
| `ts` | TEXT | ISO-8601 UTC timestamp |
| `cpu_percent`, `memory_percent` | REAL | Utilization percentages |
| `memory_used_bytes` | INTEGER | Used memory in bytes |
| `net_sent_bytes`, `net_recv_bytes` | INTEGER | **Per-sample deltas**, not cumulative counters |
| `gpu_percent`, `gpu_temperature_c` | REAL | Optional NVIDIA telemetry |
| `gpu_memory_used_mb` | REAL | Optional GPU memory in MiB (note: MiB, not bytes) |

`settings` is a key/value table (`key TEXT PRIMARY KEY, value TEXT NOT NULL`). The plan's data-model table uses descriptive placeholder names such as `timestamp`, `app_name`, `command_name`, `working_directory`, `metadata_json`, `network_rx_bytes`, and `gpu_memory_used_bytes`; the implemented schema uses the shorter names above and stores GPU memory in MiB.

`/api/summary` notes: `date` and the `date` filters on `/api/events` and `/api/metrics` select one **local calendar day** (midnight to midnight in the system timezone, converted to UTC for the query). `sites[].visits` counts `tab_focus` events per domain — the number of times the tab became active — while `sites[].seconds` accumulates `tab_session` durations; the two dimensions are not added together.

## API

All endpoints are under `http://127.0.0.1:8765`.

| Method | Endpoint | Notes |
|---|---|---|
| `GET` | `/api/health` | Process health, pause state, event count, per-collector status |
| `GET` | `/api/token` | Local token bootstrap (blocked for untrusted browser origins) |
| `GET` | `/api/summary?date=YYYY-MM-DD` | Daily summary with local-day boundaries |
| `GET` | `/api/events?limit=100&before_id=…&source=…&date=YYYY-MM-DD` | Newest-first events, limit + id cursor |
| `POST` | `/api/events` | Ingest one event; requires `X-API-Token` |
| `GET` | `/api/metrics?limit=120&before_id=…&date=YYYY-MM-DD` | Metric samples oldest-first |
| `GET` | `/api/settings` | Privacy/retention configuration |
| `POST` | `/api/settings` | Update settings; requires `X-API-Token` |
| `POST` | `/api/control` | Pause/resume; requires `X-API-Token` |
| `POST` | `/api/control/delete-all` | Delete all events and metrics; requires `X-API-Token` |

Example event:

```bash
curl -X POST http://127.0.0.1:8765/api/events \
  -H "X-API-Token: $ACTIVITY_MONITOR_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"source":"firefox","event_type":"tab_focus","url":"https://example.com/page","title":"Example"}'
```

## Troubleshooting

- Backend health: `curl http://127.0.0.1:8765/api/health`
- Collector status is included in the health response. A collector with `"ok": false` reports `last_error`.
- Hyprland: `hyprctl activewindow -j`. If `HYPRLAND_INSTANCE_SIGNATURE` is unset, you are not in a Hyprland session.
- Firefox: inspect the extension at `about:debugging#/runtime/this-firefox`.
- fish: `fish -n fish/activity-monitor.fish` checks syntax. Recording uses the `fish_preexec` hook, which fires **only for interactive command lines** — `fish -c '…'`, scripts, and stdin-fed sessions do not record anything; test the hook in a real interactive session.
- systemd logs: `journalctl --user -u activity-monitor.service -f`.
- Idle/lock hints: `loginctl show-session "$XDG_SESSION_ID" -p Active -p IdleHint -p LockedHint`. If these stay `no` while locked, Hyprland/hyprlock is not publishing the hint; tracking continues unless the session becomes inactive.
- API token: `curl http://127.0.0.1:8765/api/token` (returns 403 only when called from an untrusted browser origin).
- Custom database: `ACTIVITY_MONITOR_DB=/path/to/activity.db`.

## Known limitations

- Hyprland only: the window collector polls `hyprctl`. On niri, GNOME, KDE, or other compositors the collector reports unhealthy and records no window events. A niri/other-compositor collector is not implemented.
- Duration totals are estimates based on a 2-second poll and can miss very short windows or shutdown boundaries.
- Idle/lock detection is best-effort via systemd-logind. The Hyprland collector treats the session as unavailable when `loginctl` reports `Active=no`, `IdleHint=yes`, or `LockedHint=yes`; if logind does not expose those hints (for example, a compositor that does not set them), tracking continues.
- Suspend/resume is not recorded as explicit events. Hyprland durations use `time.monotonic()`, so time spent suspended is not counted, but a suspend boundary is not visible in the timeline.
- The Hyprland collector polls every 2 seconds, so idle/lock detection can lag a transition by up to one poll interval.
- Firefox temporary add-ons must be reloaded after browser restarts.
- fish command attribution is approximate for aliases, functions, pipelines, and compound commands.
- GPU metrics require `nvidia-smi`.
- Dashboard metric charts render the latest up to 500 samples of the selected day. At the default 5-second interval that covers roughly the last 40 minutes of a full past day; raise the metrics interval for finer long-range history, or use the default view for live monitoring.
- The API token is a local bootstrap mechanism, not a multi-user authorization system. Any installed Firefox extension can present a `moz-extension://` origin and read the token or post events; only add trusted extensions to the profile this dashboard runs in.

## Repository layout

```text
activity-monitor/
├── backend/activity_monitor/   # FastAPI app, DB, collectors
├── backend/tests/              # pytest suite
├── frontend/                   # React/Vite/Tailwind dashboard
├── firefox-extension/          # WebExtension (manifest v2)
├── fish/                       # fish preexec hook
├── scripts/test.sh             # repeatable checks
└── systemd/                    # user service template
```
