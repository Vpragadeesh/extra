import { ShieldCheck } from 'lucide-react';
import Toggle from '../components/Toggle';

// Per-field validation shared by the Privacy view and App. Exported for tests.
// Returns a map field_name -> message; empty map = valid.
export function validatePrivacyDraft(draft) {
  const errors = {};
  if (!draft) return errors;

  const lines = (value) => String(value || '').split('\n').map((s) => s.trim()).filter(Boolean);

  const domainLines = lines(draft.excluded_domains);
  if (domainLines.some((line) => /[\s,]/.test(line))) {
    errors.excluded_domains = 'Use one domain per line (no commas or spaces), e.g. "example.com".';
  }
  const appLines = lines(draft.excluded_apps);
  if (appLines.some((line) => /[\s]/.test(line))) {
    errors.excluded_apps = 'Use one application name per line, e.g. "keepassxc".';
  }

  const retention = Number(draft.retention_days);
  if (!Number.isInteger(retention) || retention < 1 || retention > 3650) {
    errors.retention_days = 'Whole number between 1 and 3650.';
  }

  const metricsInterval = Number(draft.metrics_interval_seconds);
  if (!Number.isInteger(metricsInterval) || metricsInterval < 2 || metricsInterval > 300) {
    errors.metrics_interval_seconds = 'Whole number between 2 and 300 seconds.';
  }

  return errors;
}

function FieldError({ id, message }) {
  if (!message) return null;
  return (
    <p role="alert" id={id} className="mt-1.5 text-xs text-rose-300">
      {message}
    </p>
  );
}

export default function PrivacyView({
  settings,
  draft,
  setDraft,
  errors,
  showToken, setShowToken,
  tokenCopied, copyToken,
  saveSettings, deleteAll,
  apiToken,
}) {
  if (!draft) {
    return (
      <section className="mt-6 rounded-2xl border border-white/[.08] bg-white/[.035] p-5 sm:p-6">
        <p className="text-sm text-slate-500">Privacy settings will load when the backend is available.</p>
      </section>
    );
  }

  const fieldTone = (invalid) => (invalid ? 'border-rose-400/50' : 'border-white/10');

  return (
    <section className="mt-6 rounded-2xl border border-white/[.08] bg-white/[.035] p-5 sm:p-6" aria-labelledby="privacy-title">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="text-sky-300"><ShieldCheck size={18} /></span>
          <h2 className="font-semibold text-slate-100" id="privacy-title">Privacy and retention</h2>
        </div>
        <span className="text-xs text-slate-500">Applies to every collector</span>
      </div>

      <div className="grid gap-6 xl:grid-cols-[1.2fr_1fr]">
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label htmlFor="excluded-domains" className="text-sm text-slate-400">
              Excluded domains (one per line)
            </label>
            <textarea
              id="excluded-domains"
              value={draft.excluded_domains}
              onChange={(e) => setDraft({ ...draft, excluded_domains: e.target.value })}
              rows={5}
              placeholder="example.com"
              aria-invalid={Boolean(errors.excluded_domains) || undefined}
              aria-describedby={errors.excluded_domains ? 'excluded-domains-error' : undefined}
              className={`mt-2 w-full rounded-xl border bg-black/20 px-3 py-2 font-mono text-xs text-slate-200 outline-none focus:border-sky-300/40 ${fieldTone(errors.excluded_domains)}`}
            />
            <FieldError id="excluded-domains-error" message={errors.excluded_domains} />
          </div>
          <div>
            <label htmlFor="excluded-apps" className="text-sm text-slate-400">
              Excluded applications (one per line)
            </label>
            <textarea
              id="excluded-apps"
              value={draft.excluded_apps}
              onChange={(e) => setDraft({ ...draft, excluded_apps: e.target.value })}
              rows={5}
              placeholder="keepassxc"
              aria-invalid={Boolean(errors.excluded_apps) || undefined}
              aria-describedby={errors.excluded_apps ? 'excluded-apps-error' : undefined}
              className={`mt-2 w-full rounded-xl border bg-black/20 px-3 py-2 font-mono text-xs text-slate-200 outline-none focus:border-sky-300/40 ${fieldTone(errors.excluded_apps)}`}
            />
            <FieldError id="excluded-apps-error" message={errors.excluded_apps} />
          </div>
        </div>

        <div className="grid gap-3">
          <div className="rounded-xl border border-white/[.06] bg-white/[.02] px-3 py-2.5">
            <label htmlFor="retention-days" className="flex items-center justify-between gap-4 text-sm text-slate-300">
              <span>Retention (days)</span>
              <input
                id="retention-days"
                type="number"
                min="1"
                max="3650"
                value={draft.retention_days}
                onChange={(e) => setDraft({ ...draft, retention_days: e.target.value })}
                aria-invalid={Boolean(errors.retention_days) || undefined}
                aria-describedby={errors.retention_days ? 'retention-days-error' : undefined}
                className={`w-24 rounded-lg border bg-black/20 px-2 py-1 text-right text-sm text-slate-200 outline-none ${fieldTone(errors.retention_days)}`}
              />
            </label>
            <FieldError id="retention-days-error" message={errors.retention_days} />
          </div>

          <div className="rounded-xl border border-white/[.06] bg-white/[.02] px-3 py-2.5">
            <label htmlFor="metrics-interval" className="flex items-center justify-between gap-4 text-sm text-slate-300">
              <span>Metrics interval (seconds)</span>
              <input
                id="metrics-interval"
                type="number"
                min="2"
                max="300"
                value={draft.metrics_interval_seconds}
                onChange={(e) => setDraft({ ...draft, metrics_interval_seconds: e.target.value })}
                aria-invalid={Boolean(errors.metrics_interval_seconds) || undefined}
                aria-describedby={errors.metrics_interval_seconds ? 'metrics-interval-error' : undefined}
                className={`w-24 rounded-lg border bg-black/20 px-2 py-1 text-right text-sm text-slate-200 outline-none ${fieldTone(errors.metrics_interval_seconds)}`}
              />
            </label>
            <FieldError id="metrics-interval-error" message={errors.metrics_interval_seconds} />
          </div>

          <Toggle label="Strip URL query strings and fragments" checked={draft.strip_query_fragments} onChange={(v) => setDraft({ ...draft, strip_query_fragments: v })} />
          <Toggle label="Store domains only (omit full URLs)" checked={draft.domain_storage_mode === 'domain_only'} onChange={(v) => setDraft({ ...draft, domain_storage_mode: v ? 'domain_only' : 'full' })} />
          <Toggle label="Omit window and page titles" checked={draft.exclude_titles} onChange={(v) => setDraft({ ...draft, exclude_titles: v })} />
          <Toggle label="Store fish working directory (cwd)" checked={draft.store_cwd} onChange={(v) => setDraft({ ...draft, store_cwd: v })} />

          <div className="flex flex-wrap gap-2">
            <button onClick={saveSettings} disabled={!apiToken || Object.keys(errors).length > 0} className="cursor-pointer rounded-xl bg-sky-300 px-4 py-2 text-sm font-semibold text-slate-950 transition hover:bg-sky-200 disabled:cursor-not-allowed disabled:opacity-40">Save settings</button>
            <button onClick={deleteAll} disabled={!apiToken} className="flex cursor-pointer items-center gap-2 rounded-xl border border-rose-300/20 bg-rose-400/10 px-4 py-2 text-sm font-semibold text-rose-200 transition hover:bg-rose-400/20 disabled:cursor-not-allowed disabled:opacity-40">Delete all data</button>
          </div>
        </div>

        <div className="sm:col-span-2 xl:col-span-2">
          <div className="flex flex-wrap items-center gap-3 rounded-xl border border-white/[.06] bg-white/[.02] px-3 py-2.5">
            <span className="text-xs text-slate-400">Local API token (use with the fish hook and extension):</span>
            <code className="min-w-0 flex-1 truncate rounded bg-black/25 px-2 py-1 font-mono text-xs text-slate-300">{apiToken ? (showToken ? apiToken : '••••••••••••••••••••••••••••') : 'loading…'}</code>
            <button onClick={() => setShowToken(!showToken)} className="cursor-pointer rounded-md px-1 text-xs text-slate-400 transition hover:text-sky-300">{showToken ? 'Hide' : 'Show'}</button>
            <button onClick={copyToken} className="flex cursor-pointer items-center gap-1 rounded-md px-1 text-xs text-slate-400 transition hover:text-sky-300">{tokenCopied ? 'Copied' : 'Copy'}</button>
          </div>
          {settings && (
            <p className="mt-2 text-xs text-slate-500">
              Saved retention: {settings.retention_days} days. Metrics every {settings.metrics_interval_seconds}s.
              Exclusions and cwd omission are applied before persistence; the Firefox extension also filters locally.
            </p>
          )}
        </div>
      </div>
    </section>
  );
}
