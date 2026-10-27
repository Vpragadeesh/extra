export default function Toggle({ label, checked, onChange, describeId, error }) {
  const describedBy = error ? describeId : undefined;
  const invalid = Boolean(error);
  return (
    <label
      className={`flex cursor-pointer items-center justify-between gap-4 rounded-xl border bg-white/[.02] px-3 py-2.5 text-sm text-slate-300 ${invalid ? 'border-rose-400/40' : 'border-white/[.06]'}`}
    >
      <span>{label}</span>
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        aria-invalid={invalid || undefined}
        aria-describedby={describedBy}
        className="h-4 w-4 accent-sky-400"
      />
    </label>
  );
}
