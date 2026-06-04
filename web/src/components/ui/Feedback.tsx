export function Spinner() {
  return <span className="spinner" />;
}

export function LoadingDots() {
  return (
    <div className="loading-dots">
      <span /><span /><span />
    </div>
  );
}

export function LoadingOverlay() {
  return (
    <div className="loading-overlay">
      <LoadingDots />
    </div>
  );
}

export function EmptyState({ icon = '📭', message = '暂无数据' }: { icon?: string; message?: string }) {
  return (
    <div className="empty">
      <div className="empty-icon">{icon}</div>
      <p>{message}</p>
    </div>
  );
}
