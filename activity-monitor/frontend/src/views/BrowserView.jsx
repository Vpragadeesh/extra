import { Globe } from 'lucide-react';
import Empty from '../components/Empty';
import Timeline from '../components/Timeline';
import { RowsSkeleton } from '../components/Skeletons';
import { fmtDuration } from '../lib/format';

export default function BrowserView({ summary, events, loading }) {
  const siteRows = summary?.sites || [];
  const browserEvents = events.filter((event) => event.source === 'firefox').slice(0, 24);

  return (
    <div className="mt-6 grid gap-6 xl:grid-cols-2">
      <section className="rounded-2xl border border-white/[.08] bg-white/[.035] p-5 sm:p-6" aria-labelledby="websites-title">
        <div className="mb-4 flex items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <span className="text-sky-300"><Globe size={18} /></span>
            <h2 className="font-semibold text-slate-100" id="websites-title">Websites</h2>
          </div>
          <span className="text-xs text-slate-500">Firefox active-tab sessions</span>
        </div>
        {loading ? (
          <RowsSkeleton rows={5} />
        ) : siteRows.length ? (
          <div className="space-y-1">
            {siteRows.slice(0, 8).map((site, i) => (
              <div key={`${site.domain}-${i}`} className="flex items-center justify-between gap-4 rounded-xl px-3 py-3 transition hover:bg-white/[.035]">
                <div className="flex min-w-0 items-center gap-3">
                  <div className="rounded-lg bg-sky-300/10 p-2 text-sky-200"><Globe size={16} /></div>
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">{site.domain}</div>
                    <div className="mt-1 text-xs text-slate-500">{site.visits} recorded sessions</div>
                  </div>
                </div>
                <div className="shrink-0 text-sm text-slate-300">{fmtDuration(site.seconds)}</div>
              </div>
            ))}
          </div>
        ) : (
          <Empty
            text="No website activity recorded for this day."
            action="Install the Firefox extension and browse normally to see session data here."
          />
        )}
      </section>

      <Timeline
        icon={Globe}
        title="Browser events"
        detail="Firefox tab activity"
        id="browser-events"
        events={browserEvents}
        loading={loading}
        emptyText="No Firefox tab events yet."
        emptyAction="Once the extension posts tab_focus/tab_session events, they stream in here."
      />
    </div>
  );
}
