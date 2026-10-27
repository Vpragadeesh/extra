import { ArrowDownRight } from 'lucide-react';

export default function Empty({ text, action }) {
  return (
    <div className="flex min-h-36 flex-col items-center justify-center text-center text-sm text-slate-500">
      <ArrowDownRight size={22} className="mb-2 opacity-40" />
      <span className="max-w-xs">{text}</span>
      {action && <p className="mt-2 max-w-xs text-xs text-slate-400">{action}</p>}
    </div>
  );
}
