export default function ErrorBanner({ title, detail, troubleshooting }) {
  return (
    <div
      role="alert"
      className="mt-6 rounded-xl border border-amber-300/20 bg-amber-300/[.06] px-4 py-3 text-sm text-amber-100"
    >
      <p>{title}</p>
      {detail && <p className="mt-2 text-xs text-amber-200/70">{detail}</p>}
      {troubleshooting && (
        <p className="mt-2 text-xs text-amber-200/70">
          Troubleshooting: check <code className="rounded bg-black/20 px-1">systemctl --user status activity-monitor</code> and <code className="rounded bg-black/20 px-1">curl http://127.0.0.1:8765/api/health</code>.
        </p>
      )}
    </div>
  );
}
