import { AppWindow, Globe, Terminal as TerminalIcon } from 'lucide-react';
import SectionTitle from './SectionTitle';
import Empty from './Empty';
import { RowsSkeleton } from './Skeletons';
import { time } from '../lib/format';

export const sourceIcon = (source) => (source === 'firefox' ? Globe : source === 'fish' ? TerminalIcon : AppWindow);

export default function Timeline({ icon: Icon, title, detail, events, emptyText, emptyAction, loading, id }) {
  return (
    <section className="rounded-2xl border border-white/[.08] bg-white/[.035] p-5 sm:p-6" aria-labelledby={id}>
      <SectionTitle icon={Icon} title={title} detail={detail} id={id} />
      {loading ? (
        <RowsSkeleton rows={4} />
      ) : events.length ? (
        <div className="space-y-1">
          {events.map((event) => {
            const RowIcon = sourceIcon(event.source);
            return (
              <div key={event.id} className="flex items-start gap-3 rounded-xl px-2 py-3 transition hover:bg-white/[.03]">
                <div className="mt-0.5 rounded-lg bg-white/[.05] p-2 text-slate-400">
                  <RowIcon size={15} />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm text-slate-200">{event.domain || event.app || event.command || event.event_type}</div>
                  <div className="mt-1 truncate text-xs text-slate-500">{event.title || event.cwd || event.source} · {String(event.event_type).replaceAll('_', ' ')}</div>
                </div>
                <div className="shrink-0 pt-0.5 text-xs text-slate-500">{time(event.ts)}</div>
              </div>
            );
          })}
        </div>
      ) : (
        <Empty text={emptyText} action={emptyAction} />
      )}
    </section>
  );
}
