# Ledger — frontend hardening

Setup 2026-10-27: pnpm-managed frontend; added @fontsource/fira-sans(+code) 5.3.0 deps,
vitest 5.0.5 + jsdom 30.1.3 + @testing-library/react 16.3.3 + jest-dom 7.0.1 devDeps.
Lockfiles: pnpm-lock.yaml updated (source of truth); npm package-lock.json stays stale — noted to user.
No worktree: uncommitted audit fixes live in-tree; isolation would fork them.

- [x] T1 tokens (tailwind.config.js, index.css) — semantic ink./accent/ok tokens, fontFamily.sans/mono, :focus-visible ring, prefers-reduced-motion kill-switch, .skeleton pulse, dark scrollbar. Additive only.
- [x] T2 lib/format.js — localDay/fmtDuration/fmtBytes/time (NaN-guarded).
- [x] T3 api.js — BASE, ApiError, request(), getToken, getSettings, saveSettings, loadDashboard (health/summary/events-80/metrics-500), pauseTracking, deleteAllData.
- [x] T4 components — MetricCard (loading prop), SectionTitle (id prop), Empty (action line), Toggle, ErrorBanner (role=alert), Skeletons (Card/MetricGrid/Panel/Rows), SystemCharts.jsx, Timeline.jsx (sourceIcon export).
- [x] T5 views — Overview/Browser/Terminal/System/Privacy (+ exported validatePrivacyDraft, per-field FieldError aria-invalid + aria-describedby).
- [x] T6 App.jsx (hash routing, refresh interval off/5/10/30/60 in `am:refreshInterval`, aria status/alert), main.jsx (mount + fontsource), index.html (title/desc/theme-color), src/test-setup.js.
- [x] T7 build green — vite build ok; fonts emit as local woff2/woff; chunk-size warning pre-existing (recharts), left as-is.
- [x] T8 tests — 17 tests in format.test.jsx / api.test.jsx / App.test.jsx (offline alert, per-field privacy errors + disabled Save, aria-describedby/aria-invalid assertions). 17/17 green. jsdom quirks handled: time() NaN guard, 12h am/pm regex, request() method 'GET'.
- [x] T9 scripts/test.sh — frontend-test step added with pnpm→npm fallback. Full gate green: 30 backend + 17 frontend + build + fish -n + node --check + manifest JSON.
- [x] T10 README/todo parity — README Frontend/Tests rewritten (pnpm, structure map, a11y/motion/responsive notes); todo.md baseline 30+17, test item checked (interactions remaining), next-steps item 5 marked done; CORS-preflight bullet restored after accidental replace.
- [x] T11 responsive — CDP screenshots vp-{375,768,1024,1440}.png in /tmp/opencode/am-audit/: no horizontal overflow, 1→2→4-col metric grid, sidebar ≥1024 (lg), charts live, empty-state action text renders, Fira Sans loaded.
- [x] T12 reviewer pass — reviewer (fresh-context) verdict: With fixes. Important fixed: (1) api.js `date=undefined` guard actually broken → conditional query strings + `loadDashboard({})` regression test; (2) refresh sequencing → `refreshRun` ref, superseded runs skip state writes, busy/firstLoad only cleared by newest run. Docs fixed: todo.md stale parentheticals for shared-api-client / skeletons / refresh-interval corrected (implemented 2026-10-27, staying open for live sign-off). Minor fixed: double settings fetch on mount removed, unknown initial hash normalized, reduced-motion now tracked via matchMedia `change` listener, App.test uses `vi.stubGlobal`, mobile nav distinct aria-label, dead local `ToggleRow` replaced by shared `Toggle` component. Deferred (noted in todo next-steps): `useMetricSeries` hook to dedupe chart mappers in Overview/System views, MetricCard unused `loading`/`hasValue` props, interval-picker + hash-nav interaction tests. Final gate: 30 backend + 18 frontend green, build green.
