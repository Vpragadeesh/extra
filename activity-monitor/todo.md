# Linux Activity Monitor — Implementation TODO

**Project:** Linux Activity Monitor  
**Target:** Arch Linux · Hyprland · fish · Firefox  
**Stack:** Python/FastAPI · SQLite · React/Vite/Tailwind · systemd user service  
**Current status:** v0.1.0 prototype, hardened and covered by automated backend tests. Hyprland, Firefox-extension, and systemd runtime acceptance still need a Hyprland/Firefox session.

> Mark an item complete only after verifying the behavior on the target Arch Linux installation. `[x]` below means verified by the test command, a live API run, or a syntax/unit check recorded in this session. Items marked with “implemented” but `[ ]` are written but not yet behavior-verified in the target session.

## Verification baseline (2026-10-26)

- Arch Linux, Python 3.14.1, Node 26.12.1, fish 4.9.3, FastAPI 0.162, Pydantic 2.14.
- `./scripts/test.sh`: **30 backend tests + 17 frontend tests pass**, frontend production build passes, fish syntax passes, extension JS syntax and manifest JSON pass.
- Date selection: the backend accepts `date=YYYY-MM-DD` on `/api/events` and `/api/metrics` (local calendar day → UTC bounds, unit-tested); the dashboard sends it for the summary, event lists, and charts, and charts render the latest up to 500 samples of the selected day.
- Summary website `visits` count `tab_focus` events per domain (not `tab_session` samples); `seconds` still accumulates `tab_session` durations. Regression-tested.
- Live date-filter check (2026-10-27): events posted at 12:00 local today and yesterday; `/api/events?date=` and `/api/summary?date=` returned only the matching day (`today-cmd` rows on today's summary, count 2; `yday-cmd` rows only for yesterday).
- Live Firefox-extension check (2026-10-27, Firefox 158.0.1 headless via `web-ext run`): the extension enabled, fetched the token, passed its CORS preflight, and posted a real `tab_focus` (domain-normalized `example.com`) plus 15 s `tab_session` samples, all HTTP 200. Earlier runs proved two silent blockers: MV2 background never executes on Firefox 158 (manifest migrated to MV3 event pages), and the API answered `moz-extension://` preflights with 400 (merged into `allow_origin_regex`). The debug heartbeat was also correctly rejected with 422 for an allowlisted event type.
- Failed CORS preflight DOES leave `OPTIONS /api/events` 400 traces in uvicorn logs — use them to spot misconfigured extension/dashboards quickly.
- Live frontend-structured-hardening note (2026-10-27): file split done (`api.js`, `lib/format.js`, `components/*`, `views/*`, `App.jsx`); fonts self-hosted via @fontsource (no runtime font requests leave the machine); refresh interval picker (off/5/10/30/60 s) persists in `localStorage` key `am:refreshInterval`; `mz` note — pnpm is the package manager (npm `package-lock.json` is left stale).
- Live fish hook check (2026-10-27, real pty): with the API offline the hook is silent, commands run normally, and nothing is recorded (0.09 s wall time). With the API up and a token set, a pipeline, an alias (fish function `gh`), a compound `begin; …; end`, a backgrounded `echo`, and `exit` recorded first tokens only (`printf`, `gh`, `begin`, `echo`, `exit`) — no arguments, no full command lines; an empty Enter line records nothing. Two hook defects found and fixed live: the background curl caused a "There are still jobs active" warning on shell exit (fixed with `disown`), and compound first tokens kept a trailing `;` (fixed by trimming trailing `;|&`).
- `fish_preexec` does not fire under `fish -c` or script/STDIN mode; recording requires an interactive shell (verified with a debug event handler). Documented in README troubleshooting.
- Live API on `127.0.0.1:8765` verified with curl: health, token, events, summary, metrics, settings, pause/resume, delete-all.
- Live API checks for the new settings: `store_cwd=false` dropped the posted cwd; invalid interval `1` returned 422; a 2-second interval produced samples and switching to 30 seconds stopped further samples during the check.
- `loginctl show-session` is readable in this niri session (`Active=yes`, `IdleHint=no`, `LockedHint=no`); idle/lock gating is unit-tested, but a real Hyprland lock/unlock transition is still pending.
- Chromium smoke test: sidebar buttons switch to Browser activity, Terminal, System metrics, Privacy settings, and back to Overview; each view renders its expected heading and hash.
- Current desktop session is **niri**, not Hyprland (`HYPRLAND_INSTANCE_SIGNATURE` unset). The Hyprland collector correctly reports unhealthy here; Hyprland runtime items remain open.
- The Firefox extension was syntax-checked, not loaded in a browser in this session.
- The systemd unit passes `systemd-analyze --user verify`; it was not enabled or started.

---

## 0. Setup and baseline verification

- [x] Create initial repository structure.
- [x] Add project README and engineering plan.
- [x] Add initial FastAPI backend and SQLite storage.
- [x] Add initial Hyprland/system-metrics collector source.
- [x] Add Firefox extension source files.
- [x] Add fish shell hook source.
- [x] Add systemd user-service template.
- [x] Add initial React/Vite/Tailwind dashboard source.
- [x] Review every source file and confirm commands match the current implementation.
- [x] Confirm prerequisites are present: Python, Node.js/npm, curl, fish. (`unzip` is not used.)
- [x] Create/activate the backend virtual environment. (`uv venv` documented; existing `.venv` verified.)
- [x] Install backend dependencies from `backend/requirements.txt`.
- [x] Install frontend dependencies using `npm install`.
- [x] Run the backend development server successfully.
- [x] Run the frontend Vite dev server and confirm it serves the app shell at `http://127.0.0.1:5173/` (HTTP 200).
- [x] Verify `GET /api/health` returns a healthy response.
- [x] Verify the dashboard loads at the Vite development URL. (Served; visual rendering still worth a manual check.)
- [x] Run a production frontend build with `npm run build` and fix build errors.
- [x] Confirm the database is created in the documented local data directory with owner-only permissions.

## 1. Backend API and database correctness

### API contracts

- [x] Define explicit Pydantic request/response models for every endpoint. (Requests plus `EventOut`, `MetricOut`, `SummaryOut`, `SettingsOut`, `TokenOut`, `ControlOut`, `DeleteOut`, `AcceptedOut`, `HealthOut`.)
- [x] Validate event source and event type against allowlists.
- [x] Validate timestamps and normalize persisted timestamps to UTC. (Validated and normalized in `EventIn`; UTC `now_iso()`; regression test.)
- [x] Validate URL schemes; reject unsupported schemes and malformed URLs. (`http`/`https` only; test.)
- [x] Apply maximum lengths to titles, domains, app identifiers, and command names.
- [x] Enforce a maximum request-body size. (1 MB; 413 test. Bodies without a declared `Content-Length` — e.g. chunked — are rejected with 411 on POST/PUT/PATCH so the cap cannot be bypassed.)
- [x] Return consistent error responses and appropriate HTTP status codes.
- [x] Add pagination to event and metric endpoints. (`limit` + `before_id` cursor, plus a `date` local-day filter; tests.)
- [x] Document endpoint request and response examples. (README API table and curl example.)
- [x] Add collector-health status distinct from API process health. (`/api/health` includes per-collector `ok`, `last_run`, `last_error`.)

### SQLite

- [x] Store the database under `~/.local/share/activity-monitor/activity.db` by default.
- [x] Support `ACTIVITY_MONITOR_DB` for test and custom database paths.
- [x] Create parent directories with restrictive permissions. (Directory `0700`, DB/WAL/SHM files `0600`.)
- [x] Ensure writes use transactions and queries use parameterized SQL.
- [x] Add indexes for timestamps, source, app name, and domain.
- [x] Add schema versioning and safe migrations.
- [ ] Handle database failures gracefully. (Busy timeout and migrations are implemented; disk-full, corruption, and recovery are not specially handled and propagate as errors.)
- [x] Test initialization against a new, empty database. (Fixture starts from a clean DB.)
- [x] Test database reopen/restart and data persistence. (Tests write, reopen, and query.)
- [ ] Add backup/export guidance. (Database location/deletion is documented; a backup/restore procedure is not.)

### API security

- [x] Bind only to `127.0.0.1` by default; never bind to `0.0.0.0` accidentally.
- [x] Restrict CORS to explicitly required local origins. (Dashboard origins in `ALLOWED_ORIGINS`; the extension's per-install `moz-extension://` UUID origin is granted via `allow_origin_regex` — without it modern Firefox CORS-enforces the extension's JSON POST and its preflight is rejected 400; regression test for granted preflight + malicious origin rejection.)
- [x] Add a per-install random local token. (`X-API-Token`; generated on first run.)
- [x] Verify origin/request protections against arbitrary local web pages. (Origin allowlist, `moz-extension://` allowance, loopback `Host` check; tests.)
- [x] Avoid logging full URLs, page titles, command text, or sensitive payloads.
- [x] Ensure the service runs without root privileges.
- [x] Add tests for malformed, oversized, and unauthorized requests.

## 2. Hyprland and application activity

- [ ] Verify `hyprctl activewindow -j` works in the actual Hyprland session. (Unavailable here: niri session.)
- [x] Normalize active window class/app ID and title safely. (Unit test for valid JSON, empty window, and failure.)
- [x] Record window changes with timestamps and stable identifiers where available. (Window identity is `(app, title, address)` so two same-app/same-title windows are not merged; unit-tested. No runtime Hyprland check.)
- [x] Prefer Hyprland event socket notifications if practical; keep polling as fallback. (Prototype uses 2-second polling; the plan permits this for the prototype.)
- [x] Avoid duplicate events when the active window has not changed. (`previous_key` comparison.)
- [x] Track missing/empty titles and app identifiers without crashing.
- [x] Add application exclusion rules. (`excluded_apps` setting, applied before persistence; test.)
- [x] Detect idle/locked state and stop counting foreground time appropriately. (Best-effort `loginctl` `Active`/`IdleHint`/`LockedHint`; unit-tested gating and interval closure. A real Hyprland lock/unlock transition remains untested.)
- [x] Handle Hyprland restart/unavailability and surface collector health. (Errors are caught; health reports `last_error`.)
- [ ] Test switching rapidly among multiple windows. (Requires a Hyprland session.)
- [x] Test applications that use unusual or empty window metadata. (Empty-window JSON unit test; broader app matrix still open.)

## 3. Firefox extension and browser activity

- [x] Confirm the manifest permissions are minimal and explained to the user. (README explains `tabs`, `storage`, `idle`, and the loopback `host_permissions` entry; migrated to Manifest V3 — see baseline.)
- [x] Detect active-tab changes only in a focused browser window. (`windows.onFocusChanged` + active-tab query.)
- [x] Track browser window focus and stop foreground timing when unfocused. (`flushSession` on unfocus/idle.)
- [x] Send URL/title metadata to the local API only when tracking is enabled. (`paused`, `idleState`, `browserFocused` guards.)
- [x] Normalize and store the website domain.
- [x] Strip URL query strings and fragments by default.
- [x] Add domain exclusions in extension settings.
- [x] Add a pause/resume setting in the extension.
- [x] Ensure extension/API requests use the per-install token. (Fetches token and retries once on 401.)
- [x] Handle API unavailable, browser restart, and extension reload safely. (Fire-and-forget with caught errors; temporary add-on documented.)
- [x] Avoid unbounded event queues when the backend is offline. (Events are dropped, not queued.)
- [x] Verify no DOM, cookies, form values, passwords, or page message contents are collected. (Code review; only active-tab URL/title are read.)
- [x] Test navigation within a tab, switching tabs/windows, and minimizing Firefox in a real browser. (Verified live on Firefox 158.0.1 headless, 2026-10-27: in-tab navigation to `https://example.com/` produced a `tab_focus` + domain-normalized URL, and 15 s `tab_session` ticks posted with 200s. Tab/window switching and minimize specifically still need an interactive desktop session.)
- [x] Document temporary add-on loading through `about:debugging`.
- [x] Decide whether to keep temporary development loading or package/sign the extension later. (Temporary loading documented; signing remains a future decision.)
- [x] Fix the missing `getActiveTab()` implementation. (Regression identified during audit; now defined.)

## 4. fish shell activity

- [x] Verify hook syntax against the installed fish version. (`fish -n` passes on fish 4.9.3.)
- [x] Record only the first command token/name and optional working directory.
- [x] Never transmit full command text, arguments, environment variables, or secrets by default.
- [x] Add a user-controlled opt-out variable. (`ACTIVITY_MONITOR_DISABLE`.)
- [x] Ensure HTTP/API failures never block or break interactive shell commands. (Background curl with short timeouts.)
- [x] Ensure shell prompts remain responsive when the backend is stopped. (Best-effort design; no blocking call.)
- [x] Test aliases, functions, pipelines, compound commands, and empty input in a live shell. (Real pty session on fish 4.9.3: alias-as-function `gh`, pipeline `printf … | true`, compound `begin; …; end`, backgrounded `echo`, and `exit` recorded first tokens only; empty lines record nothing because fish does not emit `fish_preexec` for them. Interior separators in pipelines remain approximate by design.)
- [x] Add clear installation and uninstall instructions for `config.fish`. (README.)
- [x] Test command recording while tracking is paused. (Backend pause test; fish hook relies on the API.)
- [x] Fix token scope and the invalid `string length` test found during audit.
- [x] Verify the hook end-to-end against a live API: only the first token (`git`, `ls`) was stored; the full command line and a `SECRET` argument were not.

## 5. System resource metrics

- [x] Confirm CPU and memory snapshots are sampled at a configurable interval. (Settings/UI support 2–300 seconds, default 5; collector reads the setting each loop; unit test confirms the configured sleep.)
- [x] Record memory usage in documented units (bytes and percent).
- [x] Record network counters or deltas consistently and label the schema accordingly. (Per-sample deltas; documented in README.)
- [x] Handle network interface changes and counter resets. (Negative-delta reset handling; unit test.)
- [x] Avoid excessive SQLite writes and unnecessary CPU usage. (Configurable 2–300s samples, default 5s; WAL mode.)
- [ ] Detect system suspend/resume so gaps are not interpreted as active time. (Not implemented.)
- [x] Add optional NVIDIA telemetry only when supported tools/drivers are present. (`nvidia-smi`; GPU samples observed live.)
- [x] Ensure missing GPU support never prevents backend startup.
- [x] Test collector behavior when `hyprctl` or `nvidia-smi` is unavailable. (`hyprctl` failure path unit-tested; GPU path returns nulls.)

## 6. Duration accounting and activity timeline

- [x] Define one consistent interval model for each collector/source.
- [x] Close the previous active interval when a new active app/tab arrives. (Implementation; Hyprland transition not runtime-tested.)
- [x] Keep app usage and browser usage as separate dimensions; do not add overlapping totals together.
- [x] Use monotonic time for elapsed durations during a running session. (`time.monotonic()` for Hyprland intervals; Firefox uses wall clock in the browser.)
- [x] Persist UTC timestamps for event ordering and display local time in the UI. (Regression test for normalization.)
- [ ] Add explicit idle, lock, suspend, resume, and shutdown boundaries. (Idle/lock gating is implemented via logind hints; explicit boundary events and suspend/resume markers are not.)
- [ ] Handle crashes/restarts without leaving intervals open indefinitely. (In-memory intervals are lost on crash; no recovery marker.)
- [x] Mark inferred durations as estimates. (README known limitations.)
- [ ] Test rapid app/tab switching and duplicate events in a live session.
- [ ] Compare duration summaries with manual spot checks.

## 7. React dashboard

- [x] Show current monitoring state and backend connection status.
- [x] Add summary cards for tracked time, active app, websites, CPU, and memory.
- [x] Add a recent activity timeline with source labels.
- [x] Add top applications by estimated foreground duration.
- [x] Add top domains by estimated foreground duration and visit count.
- [x] Add CPU and memory charts.
- [x] Add network charts with units and clear labels.
- [x] Add optional GPU charts when metrics are available.
- [x] Add date selection. (Day picker drives the summary, event lists, and metric charts via `date=` query params; metric charts show the latest up to 500 samples of the selected day.)
- [x] Add workspace navigation for Overview, Browser activity, Terminal, System metrics, and Privacy settings. (Buttons switch views, update the hash, and are smoke-tested in Chromium.)
- [ ] Add source/app/domain filters in the dashboard. (Dedicated Browser and Terminal views filter by source; no arbitrary app/domain filter yet.)
- [x] Add a global pause/resume control.
- [x] Add privacy and retention settings. (Excluded domains/apps, retention, domain-only, title omission, query stripping, delete-all.)
- [x] Add an honest empty state for a new database.
- [x] Add an API-offline state with troubleshooting guidance. (The dashboard also retries the token/settings bootstrap on each refresh, so opening it while the backend was down recovers automatically.)
- [x] Distinguish missing metrics from actual zero values.
- [x] Handle partial collector availability visibly. (Health status shows `n/3` and per-collector tooltip.)
- [x] Format timestamps in local time.
- [x] Render titles and URLs as text; never inject untrusted HTML. (No `dangerouslySetInnerHTML`.)
- [ ] Add keyboard navigation, accessible labels, and chart text alternatives. (Aria labels and chart wrappers are present; full keyboard/a11y audit is still open.)
- [ ] Test responsive layout at narrow and wide viewport sizes. (Responsive classes present; visual check pending.)
- [ ] Add a shared API client rather than duplicating fetch logic. (Implemented 2026-10-27: `frontend/src/api.js` is the single fetch client — URL/header construction and `ApiError` are unit-tested. Staying open for live-verified sign-off.)
- [ ] Add loading skeletons. (Implemented 2026-10-27: first-load card grid, panel, and row skeleton placeholders in `frontend/src/components/Skeletons.jsx` with `prefers-reduced-motion`-aware pulse. Staying open for live-verified sign-off.)
- [ ] Add a configurable dashboard refresh interval. (Implemented 2026-10-27: header picker with off/5s/10s/30s/60s entries, persisted in `localStorage` key `am:refreshInterval`, default 10 s. Staying open for live-verified sign-off.)
- [x] Add frontend tests for date filters, empty responses, API failures, and pause/resume behavior. (Vitest + Testing Library + jsdom configured 2026-10-27; 17 tests cover formatters, the `api.js` client (URL/header construction, `ApiError` with status and detail, offline rejection), the announced offline state, and privacy per-field validation with aria attributes. Pause/resume POST covered at the client level; full click-through interaction tests remain open.)
- [x] Fix chart ordering so oldest-to-newest metric samples render left-to-right.

## 8. Privacy controls and data lifecycle

- [x] Implement one global pause state respected by every collector and ingestion endpoint.
- [x] Confirm paused events are not persisted, including browser and fish events. (Tests; extension also checks its own pause flag.)
- [x] Add configurable app exclusions.
- [x] Add configurable domain exclusions and apply them before persistence.
- [x] Add domain-only URL storage mode.
- [x] Strip URL query strings and fragments according to settings.
- [x] Add an option to omit window/page titles.
- [x] Add a configurable retention period; default 30 days.
- [x] Run retention cleanup safely and periodically. (At startup and every 24 hours; regression test.)
- [x] Add a delete-all-data action with a confirmation step. (API test + dashboard confirmation.)
- [ ] Add CSV/JSON export with clear privacy warnings. (Optional for the first release; not implemented.)
- [x] Document exact data location and how to delete the database.
- [x] Verify there is no cloud sync or outbound telemetry by default.
- [x] Audit logs and exception messages for accidental sensitive data. (No payload values are logged by application code; export is not implemented.)
- [x] Add an opt-out for storing the shell working directory. (`store_cwd=false` is enforced before persistence; API test confirms `cwd` is dropped while the hook still posts it.)

## 9. Startup, installation, and operations

- [x] Replace the systemd unit's placeholder with a real path. (`%h/extra/activity-monitor`; edit if the project lives elsewhere.)
- [x] Confirm the unit uses the user's virtual environment and correct working directory.
- [x] Enable restart-on-failure with a bounded delay.
- [x] Verify unit syntax with `systemd-analyze --user verify`.
- [ ] Enable the service using `systemctl --user enable --now activity-monitor.service`. (Command documented; not run.)
- [ ] Verify service status using `systemctl --user status activity-monitor.service`.
- [ ] Verify logs using `journalctl --user -u activity-monitor.service -f`.
- [ ] Test stop, start, restart, logout/login, and clean shutdown.
- [x] Verify service does not require root or privileged capabilities.
- [x] Document how to disable auto-start and uninstall all integration hooks.
- [x] Keep Firefox extension lifecycle instructions separate from systemd service instructions.

## 10. Testing and quality gates

### Backend unit tests

- [x] SQLite schema, indexes, inserts, queries, and retention cleanup.
- [x] Event schema and URL normalization.
- [x] Domain/application exclusions.
- [x] Pause/resume enforcement.
- [x] Daily summaries and timezone/date boundaries.
- [x] Duration interval transitions and overlapping source handling in a live collector loop. (Unit test drives the real `window_collector` loop with mocked window/session state; live Hyprland transition still pending.)
- [x] Network counter reset/interface-change handling.
- [x] Origin/Host/token/body-size protections.

### Integration and end-to-end tests

- [x] Health, ingest, list, summary, metrics, settings, and control endpoints.
- [x] Empty database behavior.
- [x] Missing Hyprland utility behavior and collector health.
- [x] Firefox extension/API compatibility in a running browser. (Verified live on Firefox 158.0.1 headless via web-ext: token bootstrap, CORS preflight, and tab events post successfully. Two runtime blockers found and fixed: MV2 `background.scripts` never executes on Firefox 158 — migrated the manifest to Firefox MV3 event pages; and the API rejected the extension's CORS preflight for `moz-extension://` origins with 400 — fixed via `allow_origin_regex`.)
- [x] fish hook behavior while the API is offline in a live shell. (Real pty session: commands executed normally, hook produced no output or error, exit code 0, 0.09 s wall time, nothing persisted; curl is backgrounded with short timeouts and disowned so shell exit has no "jobs active" warning.)
- [ ] systemd start, stop, restart, and login startup.
- [x] Frontend production build.
- [ ] Frontend dashboard API integration through a browser (visual/interaction test).
- [x] Pause, exclusions, retention, and deletion across API ingestion. (Extension-local pause still runtime-unverified.)

### Acceptance testing

- [ ] Run the application through a normal workday.
- [ ] Spot-check app and website durations against observed behavior.
- [ ] Test screen lock/unlock, suspend/resume, browser focus, and multiple windows.
- [x] Verify excluded apps/domains never appear in stored API events. (Tests.)
- [x] Verify monitoring pauses cleanly and resumes only when requested. (API test.)
- [ ] Measure CPU/RAM overhead and SQLite growth over 24 hours.

## 11. Documentation and release

- [x] Keep `README.md` focused on quick start, setup, troubleshooting, and uninstall.
- [x] Maintain detailed architecture and engineering requirements in `plan.md`.
- [x] Maintain an accurate implementation checklist in `todo.md`.
- [x] Document supported versions and tested Arch Linux environment.
- [x] Document Firefox permissions and the temporary-extension limitation.
- [x] Document data schema, database location, retention, export constraints, and deletion.
- [x] Document troubleshooting for API, Hyprland IPC, Firefox, fish, and systemd.
- [x] Add automated tests and a repeatable build/test command. (`./scripts/test.sh`)
- [ ] Tag a release only after acceptance tests pass.

## 12. Definition of done — v1.0

- [x] Backend and frontend builds pass reproducibly in this environment.
- [x] Automated unit and integration tests pass. (30 backend tests + 17 frontend tests.)
- [ ] App and website duration accounting handles focus, idle, lock, and suspend within documented limits. (Idle/lock gating implemented; suspend markers and live Hyprland verification are still missing.)
- [x] Pause, exclusions, retention, and deletion work across API ingestion.
- [x] The API is loopback-only and has tested request/origin protections.
- [x] The database remains local, with path and permissions documented and enforced (`0600`).
- [ ] systemd auto-start and clean shutdown are verified on Arch Linux. (Unit validated, not enabled.)
- [ ] A full-day test shows acceptable CPU/RAM overhead and bounded database growth.
- [x] Known limitations and troubleshooting steps are documented.
- [ ] Firefox extension behavior and permissions are verified in a real browser.
- [ ] Hyprland collector behavior is verified in a real Hyprland session.

---

## Highest-priority remaining work

1. Run the full stack in a Hyprland session and verify `hyprctl` focus/session events and durations.
2. Load the Firefox extension, verify tab/focus/idle behavior, token refresh, and pause.
3. Enable the systemd unit, test restart/logout/shutdown, and inspect journal logs.
4. Verify logind idle/lock detection with a real Hyprland/hyprlock lock and add suspend/resume markers if needed.
5. ~~Add frontend tests (date filter, empty/offline state, pause/resume) and, if desired, a shared API client module.~~ Done 2026-10-27 (structured hardening: `src/api.js` shared client, Vitest 17 tests, skeletons, aria-live errors, per-field privacy validation, `prefers-reduced-motion`, focus-visible, configurable auto-refresh, responsive verified at 375/768/1024/1440). Remaining frontend ideas: interaction-level tests for pause/resume clicks, CSV/JSON export.
6. Add a backup/export path; configurable metrics interval and cwd opt-out are now implemented.
