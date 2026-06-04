interface ProgressBarProps {
  value: number;
  max: number;
  state?: 'success' | 'failed' | '';
}

export function ProgressBar({ value, max, state = '' }: ProgressBarProps) {
  const pct = max > 0 ? Math.round((value / max) * 100) : 0;
  return (
    <div className="progress-bar" style={{ height: 5 }}>
      <div className={`progress-fill ${state}`} style={{ width: `${pct}%` }} />
    </div>
  );
}
