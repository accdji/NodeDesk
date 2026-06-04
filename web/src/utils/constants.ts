export const STATUS_COLORS: Record<string, string> = {
  running: '#3B82F6',
  success: '#10B981',
  failed: '#EF4444',
  cancelled: '#F59E0B',
  skipped: '#F59E0B',
  pending: '#94A3B8',
};

export const STATUS_ICONS: Record<string, string> = {
  running: '●',
  success: '✓',
  failed: '✗',
  cancelled: '✗',
  skipped: '▶',
  pending: '●',
};

export const STEP_BADGE_CLASS: Record<string, string> = {
  success: 'badge-success',
  failed: 'badge-failed',
  running: 'badge-running',
  skipped: 'badge-skipped',
  pending: 'badge-pending',
};

export function formatDuration(seconds: number): string {
  if (!seconds || seconds <= 0) return '—';
  if (seconds < 60) return `${seconds.toFixed(0)}s`;
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}m ${s}s`;
}

export function formatTime(dateStr: string): string {
  if (!dateStr) return '—';
  const d = new Date(dateStr);
  return d.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

export function classNames(...args: (string | undefined | false | null)[]): string {
  return args.filter(Boolean).join(' ');
}
