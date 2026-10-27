export default function SectionTitle({ icon: Icon, title, detail, id }) {
  return (
    <div className="mb-4 flex items-center justify-between gap-3">
      <div className="flex items-center gap-2">
        <span className="text-sky-300"><Icon size={18} /></span>
        <h2 className="font-semibold text-slate-100" id={id}>{title}</h2>
      </div>
      {detail && <span className="text-xs text-slate-500">{detail}</span>}
    </div>
  );
}
