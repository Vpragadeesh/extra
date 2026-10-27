# Linux Activity Monitor — Product & Engineering Plan

**Project:** Local Activity Monitor  
**Initial release:** `0.1.0` (prototype)  
**Target environment:** Arch Linux, Hyprland, fish shell, Firefox  
**Frontend:** React + Vite + Tailwind CSS  
**Backend:** Python + FastAPI  
**Storage:** Local SQLite  
**Startup:** systemd user service  
**Privacy model:** Local-first; no cloud account or remote telemetry

---

## 1. Product goal

Build a transparent, user-controlled application that summarizes how the owner uses their Linux computer. It should combine foreground application usage, Firefox tab metadata, shell-command metadata, and system resource samples into one local dashboard.

The product is an activity and productivity tracker, **not** a keylogger, covert surveillance tool, or forensic recorder. It must not collect passwords, cookies, form contents, private-message contents, keystrokes, or page DOM content. Monitoring should be visible, pausable, configurable, and easy to uninstall.

### Success criteria

- A user can install and run the backend without root privileges.
- Activity data is stored in a SQLite database under the user's data directory.
- The API listens on loopback (`127.0.0.1`) only by default.
- The dashboard can show recent events, today's summary, website usage, and system metrics.
- The Firefox extension reports active-tab metadata only when enabled.
- The fish hook records limited command metadata, not full command text or arguments.
- A systemd **user** service starts the backend after login.
- The user can pause collection and delete locally stored data.

---

## 2. Scope

### Included

1. **Application/window activity**
   - Poll Hyprland's active-window information through `hyprctl`.
   - Record app class, window title, process/window identifiers when available, and timestamps.
   - Derive approximate foreground durations by closing the previous interval when a new window is detected.

2. **Firefox activity**
   - Use a Firefox WebExtension to observe active-tab changes and browser focus/idle state.
   - Record URL, page title, timestamps, and estimated foreground time.
   - Allow users to exclude domains and avoid storing URL query strings where configured.
   - Never inspect the DOM or read cookies, passwords, form values, or page message contents.

3. **Terminal activity**
   - Use a fish hook to record the executable/first command token, current working directory, and timestamp.
   - Do not submit full command text or arguments by default; commands can contain secrets.
   - Make the hook best-effort so a stopped backend does not break interactive shell use.

4. **System metrics**
   - CPU and memory snapshots.
   - Network byte counters/deltas.
   - Optional NVIDIA GPU telemetry when supported by installed drivers/utilities.
   - Sampling interval configurable to balance detail and resource overhead.

5. **Dashboard and reporting**
   - Current monitoring state and collector health.
   - Daily totals by application and website/domain.
   - Recent event timeline.
   - CPU, memory, network, and optional GPU charts.
   - Date selection and basic filtering.

6. **Local controls**
   - Pause/resume tracking.
   - App/domain exclusions.
   - Data retention and deletion controls.
   - Export selected summaries/events to CSV or JSON in a future milestone.

### Explicitly out of scope

- Capturing keystrokes, screenshots, screen recordings, passwords, cookies, or page contents.
- Covert monitoring of other people or machines.
- Uploading browsing/activity data to a cloud service by default.
- Privilege escalation or requiring root for routine collection.
- Claiming that every event is captured; short events, crashes, sleep, and shutdown may cause gaps.

---

## 3. Architecture

```text
┌──────────────────────────── User session ────────────────────────────┐
│                                                                       │
│  Hyprland collector      Firefox extension       fish shell hook      │
│  active window           active tab metadata     command metadata     │
│          │                       │                       │             │
│          └───────────────────────┴───────────────────────┘             │
│                                  │ HTTP loopback                      │
│                                  ▼                                    │
│                       Python + FastAPI backend                        │
│                     validation / pause / aggregation                  │
│                                  │                                    │
│                                  ▼                                    │
│                            SQLite database                            │
│                                  ▲                                    │
│                                  │ HTTP loopback                      │
│                       React + Vite dashboard                           │
│                                                                       │
│  systemd --user starts the backend after login                        │
└───────────────────────────────────────────────────────────────────────┘
```

### Component responsibilities

| Component | Responsibility | Failure behavior |
|---|---|---|
| Hyprland collector | Detect active-window changes | Log an error and retry; do not crash the API |
| Firefox extension | Send active-tab/focus metadata | Queue only a small bounded number of events or drop safely when API is unavailable |
| fish hook | Send command metadata | Fail silently or emit a concise debug message; never block shell use |
| FastAPI | Validate, accept, query, and summarize events | Return clear HTTP errors; never expose a public network listener by default |
| SQLite | Persist events and metric samples | Use transactions, indexes, and a busy timeout |
| React dashboard | Display summaries and controls | Show API-offline and empty states instead of misleading zeros |
| systemd user unit | Start and supervise backend | Restart on failure with a bounded delay; logs available through journalctl |

### Security boundaries

- Bind the API to `127.0.0.1`, not `0.0.0.0`.
- Add request validation and a small maximum request body size.
- Validate event types and limit string lengths.
- Restrict CORS to the local dashboard origin and required extension origin/configuration. Do not use wildcard CORS with credentials.
- Treat local web pages as untrusted: a loopback API is still potentially reachable by browser-based requests. Add an origin check and a per-install random local token or another explicit request-authentication mechanism before treating the service as hardened.
- Do not run as root. Store the database with user-only filesystem permissions where supported.
- Never log full URLs or titles to systemd logs unless debug logging is explicitly enabled.

---

## 4. Repository layout

```text
activity-monitor/
├── README.md
├── plan.md
├── backend/
│   ├── requirements.txt
│   └── activity_monitor/
│       ├── __init__.py
│       ├── __main__.py
│       ├── main.py          # FastAPI routes and app lifecycle
│       ├── db.py            # SQLite connection, schema, queries
│       └── collectors.py    # Hyprland and system metric collection
├── frontend/
│   ├── package.json
│   ├── vite.config.js
│   ├── tailwind.config.js
│   ├── postcss.config.js
│   ├── index.html
│   └── src/
│       ├── main.jsx         # Dashboard UI and API calls
│       └── index.css        # Tailwind and global styles
├── firefox-extension/
│   ├── manifest.json
│   ├── background.js
│   ├── options.html
│   └── options.js
├── fish/
│   └── activity-monitor.fish
└── systemd/
    └── activity-monitor.service
```

Keep collectors, API routes, database logic, and UI components separated as the project grows. Avoid adding unrelated responsibilities to a single large file.

---

## 5. Data model

Use SQLite in the XDG data directory by default:

```text
~/.local/share/activity-monitor/activity.db
```

Allow an override through `ACTIVITY_MONITOR_DB` for testing and advanced installs. Create parent directories with restrictive permissions where possible.

### `events`

| Field | Type | Purpose |
|---|---|---|
| `id` | INTEGER PRIMARY KEY | Event identifier |
| `timestamp` | TEXT / ISO-8601 UTC | Event time |
| `source` | TEXT | `hyprland`, `firefox`, or `fish` |
| `event_type` | TEXT | `window_change`, `tab_change`, `command`, etc. |
| `app_name` | TEXT nullable | Application identifier |
| `title` | TEXT nullable | Window/page title, subject to privacy controls |
| `url` | TEXT nullable | Firefox URL, subject to domain/query filtering |
| `domain` | TEXT nullable | Normalized website host |
| `command_name` | TEXT nullable | First command token only |
| `working_directory` | TEXT nullable | Shell working directory; allow opt-out |
| `metadata_json` | TEXT nullable | Small allowlisted metadata only; never secrets |

Recommended indexes: `(timestamp)`, `(source, timestamp)`, `(domain, timestamp)`, and `(app_name, timestamp)`.

### `metrics`

| Field | Type | Purpose |
|---|---|---|
| `id` | INTEGER PRIMARY KEY | Sample identifier |
| `timestamp` | TEXT / ISO-8601 UTC | Sample time |
| `cpu_percent` | REAL nullable | CPU utilization |
| `memory_percent` | REAL nullable | Memory utilization |
| `memory_used_bytes` | INTEGER nullable | Used memory |
| `network_rx_bytes` | INTEGER nullable | Received byte counter or delta; document which one is stored |
| `network_tx_bytes` | INTEGER nullable | Sent byte counter or delta; document which one is stored |
| `gpu_percent` | REAL nullable | Optional GPU utilization |
| `gpu_memory_used_bytes` | INTEGER nullable | Optional GPU memory use |
| `gpu_temperature_c` | REAL nullable | Optional GPU temperature |

### Duration accounting

Do not simply add durations from every collector: browser time overlaps with application time by design. Keep application time and website time as separate dimensions. For each source, close the previous active interval when a new active item arrives. Handle idle, lock, suspend, and resume as explicit boundaries. Use monotonic time for elapsed duration during a running session and UTC timestamps for persisted event ordering. Mark duration estimates as estimates.

---

## 6. API plan

The prototype currently exposes these endpoints; verify request/response schemas against the implementation before relying on them in production.

| Method | Endpoint | Purpose |
|---|---|---|
| `GET` | `/api/health` | Backend health and version |
| `GET` | `/api/summary?date=YYYY-MM-DD` | Daily summary |
| `GET` | `/api/events?limit=100` | Recent event list |
| `POST` | `/api/events` | Ingest one event |
| `GET` | `/api/metrics?limit=120` | Recent metrics |
| `POST` | `/api/control` | Pause/resume collection |

### API hardening tasks

- Define Pydantic models for every request and response.
- Validate timestamps, URL schemes, event source, enum values, and string lengths.
- Add pagination (`limit` + cursor or timestamp/ID), not just an unbounded query.
- Add a database transaction for each write and ensure queries are parameterized.
- Add local request authentication / origin protections for the extension and dashboard.
- Add a versioned API prefix if the project grows (`/api/v1`).
- Expose collector status separately from basic process health.
- Return consistent error responses and useful status codes.

---

## 7. Frontend plan

### Main dashboard

1. **Overview cards:** active app, today's tracked application time, distinct websites, current CPU/memory.
2. **Timeline:** recent app/window/tab/terminal events with source labels.
3. **App usage:** top applications by estimated foreground time.
4. **Website usage:** top domains, visit count, and estimated foreground time.
5. **System charts:** CPU, memory, network, and optional GPU history.
6. **Controls:** pause/resume, refresh interval, and collector status.
7. **Privacy settings:** excluded domains/apps, title storage, URL query stripping, retention period, and clear-data action with confirmation.

### UI states

- Loading skeletons while data is fetched.
- Explicit empty state for a new database.
- API unavailable state with troubleshooting steps.
- Partial collector availability state.
- Error state that does not silently render failures as zero activity.
- Accessible labels, keyboard navigation, and responsive layout.

### Frontend quality

- Keep API calls in a small client module rather than duplicating fetch logic.
- Avoid rendering raw untrusted HTML from titles or URLs.
- Format timestamps in the user's local timezone while storing UTC.
- Add chart labels, units, and accessible text alternatives.
- Add tests for date filters, empty responses, API failures, and pause/resume behavior.

---

## 8. Collector implementation details

### Hyprland

- Use `hyprctl activewindow -j` or the Hyprland IPC/event socket, depending on reliability needs.
- Prefer event-driven window-change notifications for responsiveness; a polling fallback can be used for the prototype.
- Normalize app class and title; do not assume every window has a useful title.
- Check whether the user is idle/locked before counting foreground time.
- Do not require elevated privileges.

### Firefox WebExtension

- Use the supported WebExtension APIs for active-tab changes and window focus.
- Request the smallest permissions possible and explain each permission in the extension options page.
- Track tab metadata only while the browser window is focused and the system is not idle.
- Normalize to a domain and optionally strip URL query/fragment values before sending.
- Add domain exclusions and a pause toggle.
- Handle browser restarts and extension reloads gracefully.
- Temporary add-ons loaded through `about:debugging` must be reloaded after Firefox restarts; document this limitation and consider a signed/distributed extension only if desired.

### fish

- Use a fish event hook appropriate for command-pre-execution or prompt events and test against the installed fish version.
- Extract only the executable/first token without transmitting the rest of the command line.
- Treat shell expansions, aliases, pipelines, and functions as cases where the “command name” may be approximate.
- Never allow a collector HTTP failure to break command execution or prompt rendering.
- Provide a simple opt-out variable and make installation/uninstallation instructions explicit.

### System metrics

- Sample at a configurable interval, initially 5–15 seconds.
- Use `psutil` for portable CPU, memory, and network counters.
- Handle counter resets and interface changes when computing network deltas.
- Query NVIDIA metrics only when the utility/driver is available; do not make NVIDIA support a startup requirement.
- Ensure metrics collection uses bounded CPU and does not write excessively to SQLite.

---

## 9. Privacy, retention, and user control

Privacy is a core requirement, not a later cosmetic feature.

- Local-only database by default; no analytics or telemetry upload.
- Loopback-only backend; never expose it to the LAN by default.
- Visible monitoring state and pause/resume control.
- Exclude sensitive applications and domains.
- Option to store domains only, omitting full URLs.
- Strip query strings and fragments by default or make this the recommended setting.
- Avoid full command lines, shell arguments, and environment variables.
- No keylogging, screenshots, page DOM inspection, cookie access, or form-content collection.
- Set a default retention period (suggested starting point: 30 days), configurable by the user.
- Provide a “delete all activity data” action with confirmation.
- Document database location and backup/export behavior.
- Add tests confirming pause stops ingestion and exclusions are applied before persistence.

**Privacy caveat:** URLs and window titles can still contain sensitive information even without page contents. For sensitive sessions, pause tracking or use domain-only mode.

---

## 10. Installation and startup

### Arch Linux packages

Required tools: Python, Node.js/npm, and the matching Hyprland utilities. Python dependencies are installed in a virtual environment. Optional GPU metrics may need `nvidia-utils` and compatible drivers.

### Development run

Backend:

```bash
cd backend
python -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt
python -m activity_monitor
```

Frontend, in a second terminal:

```bash
cd frontend
npm install
npm run dev
```

### systemd user service

- Replace the service template's placeholder project path with the actual absolute path.
- Use `systemctl --user enable --now activity-monitor.service`.
- Inspect logs with `journalctl --user -u activity-monitor.service -f`.
- Keep the service unprivileged.
- Confirm restart behavior and environment variables in the user's actual session.
- Do not claim the Firefox extension auto-starts with the backend; browser extension lifecycle is separate.

### Installation verification

1. `curl http://127.0.0.1:8765/api/health` returns healthy status.
2. `hyprctl activewindow -j` works in the Hyprland session.
3. SQLite database appears at the configured path.
4. A test event can be posted and retrieved.
5. Dashboard loads and displays an API-offline state if the backend is stopped.
6. Firefox extension successfully posts a tab event.
7. fish commands continue normally if the backend is stopped.
8. User service starts after logout/login and can be stopped cleanly.

---

## 11. Testing strategy

### Unit tests

- SQLite initialization, migrations, inserts, indexes, and retention cleanup.
- Event validation and URL normalization.
- Domain/app exclusion rules.
- Pause/resume enforcement.
- Summary aggregation and date boundaries.
- Duration interval transitions and overlapping source handling.
- Network counter reset/interface-change behavior.

### Integration tests

- FastAPI health, event ingestion, event listing, metrics, and summary routes.
- API behavior with an empty or corrupted database.
- API behavior when Hyprland/NVIDIA utilities are unavailable.
- Firefox payload validation and extension/API compatibility.
- fish hook behavior when the backend is not running.
- systemd start, stop, restart, and log inspection.

### Acceptance tests

- Run for a full workday and compare app/site totals with a manual spot check.
- Test lock/unlock, suspend/resume, browser focus changes, multiple windows, and network interface changes.
- Confirm pause means no new activity events are persisted.
- Confirm excluded domains/apps do not enter the database.
- Confirm all data remains on disk locally and no outbound telemetry is present.
- Measure CPU, memory, and database growth over 24 hours.

---

## 12. Milestones and task checklist

### Milestone 0 — Prototype baseline

- [x] Create project structure and README.
- [x] Implement initial FastAPI routes and SQLite storage.
- [x] Add initial Hyprland/system metrics collectors.
- [x] Add initial Firefox extension source.
- [x] Add initial fish hook and systemd user-service template.
- [x] Create initial React dashboard source.
- [ ] Verify frontend dependency installation and production build in target environment.
- [ ] Run full end-to-end verification on Arch Linux + Hyprland.

### Milestone 1 — Correctness and resilience

- [ ] Add automated backend tests and CI checks.
- [ ] Harden API request models, CORS/origin handling, and local authentication.
- [ ] Make collectors independently supervised and report their health.
- [ ] Improve duration accounting for idle, lock, sleep, resume, and shutdown.
- [ ] Handle database errors and migrations safely.
- [ ] Add frontend API client, loading/error/empty states, and tests.

### Milestone 2 — Privacy controls

- [ ] Add app/domain exclusions in the backend and extension settings.
- [ ] Add domain-only mode and URL query/fragment stripping.
- [ ] Add a global pause control that all collectors honor.
- [ ] Add configurable retention and delete-all-data action.
- [ ] Add clear install/uninstall and data-location documentation.

### Milestone 3 — Useful analytics

- [ ] Add accurate per-app and per-domain duration aggregation.
- [ ] Add daily/weekly date selection and top usage lists.
- [ ] Add idle time and session summaries.
- [ ] Add system resource charts and optional NVIDIA metrics.
- [ ] Add CSV/JSON export.

### Milestone 4 — Release quality

- [ ] Add database migrations and backup/restore guidance.
- [ ] Add packaging/install script with reversible changes.
- [ ] Validate systemd service on a clean Arch user account.
- [ ] Verify Firefox extension lifecycle and permissions.
- [ ] Profile long-running operation and SQLite growth.
- [ ] Tag a release and document known limitations.

---

## 13. Known limitations of v0.1.0

- It is an initial prototype, not a fully audited or production-hardened monitoring service.
- Exact duration totals depend on collector event quality and correct handling of idle/suspend boundaries.
- Firefox temporary extension installation requires reloading after browser restarts.
- The fish hook can only infer command metadata; aliases, shell functions, pipelines, and compound commands complicate attribution.
- GPU metrics depend on the installed NVIDIA driver and command-line utility.
- Short-lived windows/events can be missed by polling or during shutdown.
- The frontend production build and complete end-to-end behavior must be verified on the target machine before relying on it daily.
- The local API requires additional origin/authentication hardening before it should be considered safe against arbitrary local web pages.

---

## 14. Definition of done for v1.0

The project is ready for regular personal use when:

- [ ] Backend and frontend builds pass reproducibly.
- [ ] Automated unit and integration tests pass.
- [ ] App and website durations handle focus changes, idle, lock, and suspend correctly within documented limits.
- [ ] Pause, exclusions, retention, and deletion work across all collectors.
- [ ] The API is loopback-only and has tested request/origin protections.
- [ ] The database remains local and its path/permissions are documented.
- [ ] systemd auto-start and clean shutdown are verified on Arch Linux.
- [ ] A full-day test shows acceptable CPU/RAM overhead and bounded database growth.
- [ ] Known limitations and troubleshooting steps are documented.
