import { useMemo } from 'react';
import { Cpu, HardDrive, ArrowDownRight } from 'lucide-react';
import MetricCard from '../components/MetricCard';
import SystemCharts from '../components/SystemCharts';
import { MetricGridSkeleton } from '../components/Skeletons';
import { fmtBytes, time } from '../lib/format';

export default function SystemView({ metrics, settings, loading }) {
  const latest = metrics.length ? metrics[metrics.length - 1] : null;

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

  return (
    <>
      {loading ? (
        <MetricGridSkeleton />
      ) : (
        <div className="mt-8 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <MetricCard
            icon={Cpu}
            label="CPU"
            value={latest ? `${Math.round(Number(latest.cpu_percent) || 0)}%` : '—'}
            hint={latest ? `Sampled ${time(latest.ts)}` : 'Waiting for samples'}
          />
          <MetricCard
            icon={HardDrive}
            label="Memory"
            value={latest ? `${Math.round(Number(latest.memory_percent) || 0)}%` : '—'}
            hint={latest?.memory_used_bytes != null ? `${fmtBytes(latest.memory_used_bytes)} used` : 'Waiting for samples'}
            accent="violet"
          />
          <MetricCard
            icon={ArrowDownRight}
            label="Network TX"
            value={latest ? fmtBytes(latest.net_sent_bytes) : '—'}
            hint="Upload delta since previous sample"
            accent="green"
          />
          <MetricCard
            icon={ArrowDownRight}
            label="Network RX"
            value={latest ? fmtBytes(latest.net_recv_bytes) : '—'}
            hint="Download delta since previous sample"
            accent="amber"
          />
        </div>
      )}

      <div className="mt-6">
        <SystemCharts
          chartData={chartData}
          netData={netData}
          gpuData={gpuData}
          hasGpu={hasGpu}
          settings={settings}
          loading={loading}
        />
      </div>
    </>
  );
}
