import type { LogEntry } from '../../types';

interface LogPanelProps {
  logs: LogEntry[];
  maxHeight?: number;
}

export function LogPanel({ logs, maxHeight = 400 }: LogPanelProps) {
  if (logs.length === 0) {
    return (
      <div className="log-panel" style={{ textAlign: 'center', color: '#6C7086' }}>
        暂无日志
      </div>
    );
  }

  return (
    <div className="log-panel" style={{ maxHeight, overflowY: 'auto' }}>
      {logs.map((l, i) => (
        <div key={l._idx || i}>
          <span className="log-time">{l.timestamp}</span>{' '}
          <span className="log-step">[{l.step_name}]</span>{' '}
          <span className="log-msg">{l.message}</span>
        </div>
      ))}
    </div>
  );
}
