import { useMemo } from 'react';
import { AppWindow, Clock3, Globe, Cpu, Activity } from 'lucide-react';
import MetricCard from '../components/MetricCard';
import Empty from '../components/Empty';
import SystemCharts from '../components/SystemCharts';
import Timeline from '../components/Timeline';
import { MetricGridSkeleton } from '../components/Skeletons';
import { RowsSkeleton } from '../components/Skeletons';
import { fmtDuration, time } from '../lib/format';

export default function OverviewView({ summary, events, metrics, settings, day, today, loading }) {
  const appRows = useMemo(() => summary?.apps || [], [summary]);
  const siteRows = useMemo(() => summary?.sites || [], [summary]);
  const activeSeconds = Number(summary?.tracked_seconds) || 0;
  const siteCount = summary?.site_count ?? siteRows.length;
  const latest = metrics.length ? metrics[metrics.length - 1] : null;
  const activeApp = events.find((e) => e.source === 'hyprland' && e.event_type === 'window_focus')?.app || '—';

  const chartData = useMemo(
    () => metrics.map((m) => ({ time: time(m.ts), cpu: Number(m.cpu_percent) || 0, ram: Number(m.memory_percent) || 0 })),
    [metrics]
  );
  const netData = useMemo(
    () => metrics.slice(-60).map((m) => ({ time: time(m.ts), tx: Number(m.net_sent_bytes) || 0, rx: Number(m.net_recv_bytes) || 0 })),
    [metrics]
  );
  const hasGpu = metrics.some((m) => m.gpu_percent != null);
  const gpuData = useMemo(
    () => metrics.slice(-60).map((m) => ({
      time: time(m.ts),
      gpu: Number(m.gpu_percent) || 0,
      gpuMem: Number(m.gpu_memory_used_mb) || 0,
      gpuTemp: Number(m.gpu_temperature_c) || 0,
    })),
    [metrics]
  );
  const recent = events.slice(0, 12);
  const total = appRows.reduce((a, r) => a + (Number(r.seconds) || 0), 0) || 1;

  return (
    <>
      {loading ? (
        <MetricGridSkeleton />
      ) : (
        <div className="mt-8 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <MetricCard icon={AppWindow} label="Active window" value={activeApp} hint="Latest Hyprland focus event" />
          <MetricCard
            icon={Clock3}
            label="Tracked app time"
            value={fmtDuration(activeSeconds)}
            hint={day === today ? 'Foreground windows recorded today' : `Foreground windows on ${day}`}
            accent="violet"
          />
          <MetricCard icon={Globe} label="Websites" value={siteCount} hint="Distinct domains with browser sessions" accent="green" />
          <MetricCard
            icon={Cpu}
            label="CPU / memory"
            value={latest ? `${Math.round(Number(latest.cpu_percent) || 0)}%` : '—'}
            hint={latest ? `${Math.round(Number(latest.memory_percent) || 0)}% RAM in use` : 'Waiting for metric samples'}
            accent="amber"
          />
        </div>
      )}

      <div className="mt-6 grid gap-6 xl:grid-cols-[1.55fr_1fr]">
        <SystemCharts
          chartData={chartData}
          netData={netData}
          gpuData={gpuData}
          hasGpu={hasGpu}
          settings={settings}
          loading={loading}
        />

        <section className="rounded-2xl border border-white/[.08] bg-white/[.035] p-5 sm:p-6" aria-labelledby="most-used-apps">
          <div className="mb-4 flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <span className="text-sky-300"><AppWindow size={18} /></span>
              <h2 className="font-semibold text-slate-100" id="most-used-apps">Most-used applications</h2>
            </div>
            <span className="text-xs text-slate-500">{day === today ? 'Today' : day}</span>
          </div>
          {loading ? (
            <RowsSkeleton rows={4} />
          ) : appRows.length ? (
            <div className="space-y-5">
              {appRows.slice(0, 7).map((row, i) => {
                const sec = Number(row.seconds) || 0;
                return (
                  <div key={`${row.app}-${i}`}>
                    <div className="mb-2 flex items-center justify-between gap-3 text-sm">
                      <span className="truncate text-slate-200">{row.app || 'Unknown app'}</span>
                      <span className="shrink-0 text-slate-400">{fmtDuration(sec)}</span>
                    </div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-white/[.07]">
                      <div className="h-full rounded-full bg-gradient-to-r from-sky-400 to-blue-500" style={{ width: `${Math.max(2, (sec / total) * 100)}%` }} />
                    </div>
                  </div>
                );
              })}
            </div>
          ) : (
            <Empty
              text="App usage will show after your first window switches."
              action="Window tracking comes from the Hyprland collector — check Privacy settings if it stays empty."
            />
          )}
        </section>
      </div>

      <div className="mt-6">
        <Timeline
          icon={Clock3}
          title="Recent activity"
          detail="Latest events"
          id="recent-activity"
          events={recent}
          loading={loading}
          emptyText="Your timeline will fill as activity is collected."
          emptyAction="Start the backend service and let a collector run for a minute."
        />
      </div>
    </>
  );
}
