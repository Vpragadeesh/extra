export function CardSkeleton({ className = '' }) {
  return (
    <div className={`rounded-2xl border border-white/[.08] bg-white/[.045] p-5 ${className}`} aria-hidden="true">
      <div className="flex items-center justify-between">
        <span className="skeleton block h-4 w-20 rounded" />
        <span className="skeleton block h-9 w-9 rounded-xl" />
      </div>
      <span className="skeleton mt-4 block h-9 w-24 rounded-lg" />
      <span className="skeleton mt-2 block h-3 w-32 rounded" />
    </div>
  );
}

export function MetricGridSkeleton({ count = 4 }) {
  return (
    <div className="mt-8 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      {Array.from({ length: count }).map((_, i) => <CardSkeleton key={i} />)}
    </div>
  );
}

export function PanelSkeleton({ className = '', height = 'h-64' }) {
  return (
    <div className={`rounded-2xl border border-white/[.08] bg-white/[.035] p-5 sm:p-6 ${className}`} aria-hidden="true">
      <span className="skeleton block h-4 w-40 rounded" />
      <div className={`mt-4 ${height} grid`}>
        <span className="skeleton block rounded-xl" />
      </div>
      <span className="skeleton mt-4 block h-3 w-56 rounded" />
    </div>
  );
}

export function RowsSkeleton({ rows = 5, className = '' }) {
  return (
    <div className={`space-y-3 ${className}`} aria-hidden="true">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3 rounded-xl px-2 py-3">
          <span className="skeleton block h-8 w-8 shrink-0 rounded-lg" />
          <div className="min-w-0 flex-1">
            <span className="skeleton block h-3.5 w-1/2 rounded" />
            <span className="skeleton mt-2 block h-3 w-1/3 rounded" />
          </div>
          <span className="skeleton block h-3 w-10 rounded" />
        </div>
      ))}
    </div>
  );
}
