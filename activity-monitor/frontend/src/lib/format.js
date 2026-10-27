// Pure formatting helpers shared by every view. No React, no fetch — safe to
// unit-test directly (see src/lib/format.test.jsx).

// YYYY-MM-DD in local time, matching what the backend's ?date= expects.
export const localDay = () => {
  const d = new Date();
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 10);
};

export const fmtDuration = (seconds = 0) => {
  const s = Math.round(Number(seconds) || 0);
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
};

export const fmtBytes = (bytes = 0) => {
  const n = Number(bytes) || 0;
  if (n < 1024) return `${n} B`;
  if (n < 1048576) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1073741824) return `${(n / 1048576).toFixed(1)} MB`;
  return `${(n / 1073741824).toFixed(2)} GB`;
};

export const time = (ts) => {
  try {
    const date = new Date(ts);
    if (Number.isNaN(date.getTime())) return '—';
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  } catch {
    return '—';
  }
};
