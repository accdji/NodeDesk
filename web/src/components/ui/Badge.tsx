export function Badge({
  children,
  type = 'pending',
}: {
  children: React.ReactNode;
  type?: 'success' | 'failed' | 'running' | 'skipped' | 'pending';
}) {
  return <span className={`badge badge-${type}`}>{children}</span>;
}

export function StatusBadge({ status }: { status: string }) {
  const map: Record<string, 'success' | 'failed' | 'running' | 'skipped' | 'pending'> = {
    success: 'success',
    failed: 'failed',
    running: 'running',
    cancelled: 'skipped',
    skipped: 'skipped',
  };
  const t = map[status] || 'pending';
  const labels: Record<string, string> = {
    success: '成功',
    failed: '失败',
    running: '运行中',
    cancelled: '已取消',
    skipped: '跳过',
    pending: '等待',
  };
  return <Badge type={t}>{labels[status] || status}</Badge>;
}
