# Frontend Structured Hardening (Scope B)

Approved scope (user, 2026-10-27): split single-file `main.jsx` into `src/api.js`,
`src/views/*`, shared components + design tokens; add skeletons, aria-live errors,
per-field privacy-form errors, `prefers-reduced-motion`, focus-visible, configurable
refresh interval, responsive verification at 375/768/1024/1440; frontend tests LAST.

Constraints from planning:
- Stack unchanged: React 18 + Vite 6 + Tailwind 3 + recharts 2. No visual redesign
  (dark glass verified in ui-ux-pro-max design-system search).
- Local-first privacy: fonts must NOT fetch from Google at runtime → self-host via
  `@fontsource/fira-sans` + `@fontsource/fira-code`.
- Full behavior parity from `main.jsx`: hash views, date param, 500-metric fetch,
  token/settings bootstrap+retry, pause/resume, delete-all confirm, token copy/show,
  GPU conditional block, `fmtBytes` axis formatters, `chartData/netData/gpuData` memos.
- Package manager: pnpm (node_modules/.modules.yaml present). Lockfiles: keep both
  npm + pnpm updated if pnpm updates its lock; pnpm commands only.
- Responsive: verify via build + `terminal-browser` at 375/768/1024/1440 if workable,
  else breakpoint-class sanity documented.

## Tasks
1. T1 tokens: `tailwind.config.js` (colors/fonts/radii) + `index.css` (tokens,
   focus-visible, prefers-reduced-motion, scrollbar, base type).
2. T2 `src/lib/format.js` — pure: localDay, fmtDuration, fmtBytes, time.
3. T3 `src/api.js` — fetch client (token header, query, JSON, errors).
4. T4 shared components: MetricCard, SectionTitle, Empty, Toggle, Skeletons,
   ErrorBanner.
5. T5 views: Overview, Browser, Terminal, System (charts), Privacy (per-field errs).
6. T6 `App.jsx` (state/data/refresh interval w/ localStorage), `main.jsx` mount,
   `index.html` (title, meta).
7. T7 `pnpm build` green + fix fallout.
8. T8 vitest tests LAST: format, api, App offline/error + privacy per-field error.
9. T9 `scripts/test.sh` grows frontend-test step; full gate green.
10. T10 README + todo.md parity.
11. T11 responsive checkpoints 375/768/1024/1440.
12. T12 fresh-context review (reviewer subagent) + fixes.

## Ledger
See `ledger.md` (one line per task: status, evidence, deviations).
