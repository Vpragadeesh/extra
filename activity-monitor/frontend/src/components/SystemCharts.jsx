import { Activity } from 'lucide-react';
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import SectionTitle from './SectionTitle';
import { PanelSkeleton } from './Skeletons';
import { fmtBytes } from '../lib/format';

const TOOLTIP_STYLE = { background: '#111a2c', border: '1px solid #ffffff20', borderRadius: 12, color: '#e2e8f0' };

function ChartEmpty({ label, hint }) {
  return (
    <div className="flex h-full flex-col items-center justify-center text-center text-sm text-slate-500">
      <Activity size={28} className="mb-3 opacity-50" />
      {label}
      <br />
      <span className="mt-1 text-xs">{hint}</span>
    </div>
  );
}

function Legend({ items }) {
  return (
    <div className="mb-4 flex gap-5 text-xs">
      {items.map(([color, label]) => (
        <span key={label} className="flex items-center gap-2 text-slate-400">
          <i className={`h-2 w-2 rounded-full ${color}`} /> {label}
        </span>
      ))}
    </div>
  );
}

export default function SystemCharts({ chartData, netData, gpuData, hasGpu, settings, loading }) {
  if (loading) return <PanelSkeleton height="h-[26rem]" />;

  return (
    <section className="rounded-2xl border border-white/[.08] bg-white/[.035] p-5 sm:p-6" aria-label="System performance charts">
      <SectionTitle icon={Activity} title="System performance" detail={`Samples every ${settings?.metrics_interval_seconds ?? 5}s`} id="system-performance" />

      <Legend items={[['bg-sky-300', 'CPU %'], ['bg-violet-300', 'RAM %']]} />
      <div className="h-64 w-full" role="img" aria-label="CPU and memory usage over time">
        {chartData.length ? (
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={chartData}>
              <defs>
                <linearGradient id="cpuFill" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#7dd3fc" stopOpacity={.25} />
                  <stop offset="100%" stopColor="#7dd3fc" stopOpacity={0} />
                </linearGradient>
                <linearGradient id="ramFill" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#c4b5fd" stopOpacity={.2} />
                  <stop offset="100%" stopColor="#c4b5fd" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid stroke="#ffffff10" vertical={false} />
              <XAxis dataKey="time" tick={{ fill: '#64748b', fontSize: 11 }} axisLine={false} tickLine={false} minTickGap={30} />
              <YAxis domain={[0, 100]} tick={{ fill: '#64748b', fontSize: 11 }} axisLine={false} tickLine={false} width={32} />
              <Tooltip contentStyle={TOOLTIP_STYLE} />
              <Area type="monotone" dataKey="cpu" name="CPU %" stroke="#7dd3fc" fill="url(#cpuFill)" strokeWidth={2} isAnimationActive={false} />
              <Area type="monotone" dataKey="ram" name="RAM %" stroke="#c4b5fd" fill="url(#ramFill)" strokeWidth={2} isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        ) : (
          <ChartEmpty label="System samples will appear here" hint="Start the backend and wait a few seconds." />
        )}
      </div>

      <div className="mt-6">
        <Legend items={[['bg-emerald-300', 'Net TX (per sample)'], ['bg-rose-300', 'Net RX (per sample)']]} />
        <div className="h-40 w-full" role="img" aria-label="Network transmit and receive per sample">
          {netData.length ? (
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={netData}>
                <defs>
                  <linearGradient id="txFill" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="#34d399" stopOpacity={.2} /><stop offset="100%" stopColor="#34d399" stopOpacity={0} /></linearGradient>
                  <linearGradient id="rxFill" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="#f87171" stopOpacity={.2} /><stop offset="100%" stopColor="#f87171" stopOpacity={0} /></linearGradient>
                </defs>
                <CartesianGrid stroke="#ffffff10" vertical={false} />
                <XAxis dataKey="time" tick={{ fill: '#64748b', fontSize: 10 }} axisLine={false} tickLine={false} minTickGap={20} />
                <YAxis tick={{ fill: '#64748b', fontSize: 10 }} axisLine={false} tickLine={false} width={46} tickFormatter={fmtBytes} />
                <Tooltip contentStyle={TOOLTIP_STYLE} formatter={(v) => [fmtBytes(v), '']} />
                <Area type="monotone" dataKey="tx" name="TX" stroke="#34d399" fill="url(#txFill)" strokeWidth={1.5} isAnimationActive={false} />
                <Area type="monotone" dataKey="rx" name="RX" stroke="#f87171" fill="url(#rxFill)" strokeWidth={1.5} isAnimationActive={false} />
              </AreaChart>
            </ResponsiveContainer>
          ) : (
            <div className="flex h-full items-center justify-center text-xs text-slate-500">Network data will appear here.</div>
          )}
        </div>
      </div>

      {hasGpu && (
        <div className="mt-6">
          <Legend items={[['bg-amber-300', 'GPU %'], ['bg-orange-300', 'GPU memory (MB)'], ['bg-red-300', 'GPU temp (°C)']]} />
          <div className="h-40 w-full" role="img" aria-label="GPU utilization, memory, and temperature">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={gpuData}>
                <defs>
                  <linearGradient id="gpuFill" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="#fbbf24" stopOpacity={.2} /><stop offset="100%" stopColor="#fbbf24" stopOpacity={0} /></linearGradient>
                  <linearGradient id="gpuMemFill" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="#fb923c" stopOpacity={.2} /><stop offset="100%" stopColor="#fb923c" stopOpacity={0} /></linearGradient>
                  <linearGradient id="gpuTempFill" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="#f87171" stopOpacity={.2} /><stop offset="100%" stopColor="#f87171" stopOpacity={0} /></linearGradient>
                </defs>
                <CartesianGrid stroke="#ffffff10" vertical={false} />
                <XAxis dataKey="time" tick={{ fill: '#64748b', fontSize: 10 }} axisLine={false} tickLine={false} minTickGap={20} />
                <YAxis tick={{ fill: '#64748b', fontSize: 10 }} axisLine={false} tickLine={false} width={40} />
                <Tooltip contentStyle={TOOLTIP_STYLE} />
                <Area type="monotone" dataKey="gpu" name="GPU %" stroke="#fbbf24" fill="url(#gpuFill)" strokeWidth={1.5} isAnimationActive={false} />
                <Area type="monotone" dataKey="gpuMem" name="GPU mem" stroke="#fb923c" fill="url(#gpuMemFill)" strokeWidth={1.5} isAnimationActive={false} />
                <Area type="monotone" dataKey="gpuTemp" name="GPU temp" stroke="#f87171" fill="url(#gpuTempFill)" strokeWidth={1.5} isAnimationActive={false} />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}
    </section>
  );
}
