const TONES = {
  blue: 'bg-sky-400/10 text-sky-300',
  violet: 'bg-violet-400/10 text-violet-300',
  green: 'bg-emerald-400/10 text-emerald-300',
  amber: 'bg-amber-400/10 text-amber-300',
};

export default function MetricCard({ icon: Icon, label, value, hint, accent = 'blue', loading = false, hasValue = true }) {
  return (
    <div className="rounded-2xl border border-white/[.08] bg-white/[.045] p-5 shadow-lg shadow-black/10">
      <div className="flex items-center justify-between">
        <span className="text-sm text-slate-400">{label}</span>
        <span className={`rounded-xl p-2 ${TONES[accent] || TONES.blue}`}><Icon size={18} /></span>
      </div>
      <div className="mt-4 truncate text-3xl font-semibold tracking-tight">
        {loading ? <span className="skeleton block h-9 w-24 rounded-lg" aria-hidden="true" /> : (hasValue ? value : '—')}
      </div>
      <div className="mt-2 truncate text-xs text-slate-500">{hint}</div>
    </div>
  );
}
