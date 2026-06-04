import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useHistoryStore } from '../stores/historyStore';
import { StatusBadge } from '../components/ui/Badge';
import { EmptyState, Spinner } from '../components/ui/Feedback';
import { Pagination } from '../components/ui/Pagination';
import { ConfirmDialog } from '../components/ui/Modal';
import { useState } from 'react';
import { formatDuration, formatTime } from '../utils/constants';

export function HistoryPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const store = useHistoryStore();
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [showBatchDelete, setShowBatchDelete] = useState(false);
  const [statusFilter, setStatusFilter] = useState('');
  const [projectFilter, setProjectFilter] = useState('');
  const [searchText, setSearchText] = useState('');

  useEffect(() => { store.load(); }, []);

  const handleFilter = () => {
    store.setFilter({ status: statusFilter, project: projectFilter, q: searchText });
    setTimeout(() => store.load(), 0);
  };

  const handlePageChange = (page: number) => {
    store.load(page);
  };

  const stats = {
    total: store.total,
    success: store.runs.filter((r) => r.status === 'success').length,
    failed: store.runs.filter((r) => r.status === 'failed').length,
    avgDuration: '—',
  };

  if (store.loading && store.runs.length === 0) {
    return <div className="empty" style={{ padding: 80 }}><Spinner /><p style={{ marginTop: 12 }}>{t('common.loading')}</p></div>;
  }

  return (
    <div>
      {/* Stats */}
      <div className="grid-4" style={{ marginBottom: 20 }}>
        <div className="card stat-card">
          <div className="stat-value">{stats.total}</div>
          <div className="stat-label">总执行次数</div>
        </div>
        <div className="card stat-card">
          <div className="stat-value" style={{ color: 'var(--success)' }}>{stats.success}</div>
          <div className="stat-label">成功</div>
        </div>
        <div className="card stat-card">
          <div className="stat-value" style={{ color: 'var(--failed)' }}>{stats.failed}</div>
          <div className="stat-label">失败</div>
        </div>
        <div className="card stat-card">
          <div className="stat-value" style={{ color: 'var(--text-muted)' }}>{stats.avgDuration}</div>
          <div className="stat-label">平均耗时</div>
        </div>
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
        <div className="search-box" style={{ flex: '1 1 180px' }}>
          <span className="search-icon">🔍</span>
          <input className="form-input" placeholder={t('history.search')}
            value={searchText} onChange={(e) => setSearchText(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleFilter()} />
        </div>
        <select className="form-input" style={{ width: 120 }} value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}>
          <option value="">全部状态</option>
          <option value="success">成功</option>
          <option value="failed">失败</option>
          <option value="running">运行中</option>
          <option value="cancelled">已取消</option>
        </select>
        <input className="form-input" style={{ width: 140 }} placeholder="项目名"
          value={projectFilter} onChange={(e) => setProjectFilter(e.target.value)} />
        <button className="btn btn-outline btn-sm" onClick={handleFilter}>筛选</button>
        <button className="btn btn-outline btn-sm" onClick={() => store.load()}>刷新</button>
        {store.selectedIds.length > 0 && (
          <button className="btn btn-outline btn-sm" style={{ color: 'var(--failed)' }}
            onClick={() => setShowBatchDelete(true)}>
            批量删除 ({store.selectedIds.length})
          </button>
        )}
      </div>

      {/* Runs List */}
      {store.runs.length === 0 ? (
        <EmptyState icon="📋" message="暂无执行记录" />
      ) : (
        <>
          <div className="card">
            <table>
              <thead>
                <tr>
                  <th className="checkbox-cell">
                    <input type="checkbox" checked={store.selectedIds.length === store.runs.length && store.runs.length > 0}
                      onChange={store.toggleAll} />
                  </th>
                  <th>Run ID</th>
                  <th>{t('project.name')}</th>
                  <th>{t('project.workflow')}</th>
                  <th>{t('status.success')}</th>
                  <th>开始时间</th>
                  <th>耗时</th>
                </tr>
              </thead>
              <tbody>
                {store.runs.map((r) => (
                  <tr key={r.id} style={{ cursor: 'pointer' }} onClick={() => navigate(`/history/${r.id}`)}>
                    <td className="checkbox-cell" onClick={(e) => e.stopPropagation()}>
                      <input type="checkbox" checked={store.selectedIds.includes(r.id)}
                        onChange={() => store.toggleSelect(r.id)} />
                    </td>
                    <td><code style={{ fontSize: 12 }}>{r.id.slice(0, 16)}</code></td>
                    <td>{r.project_name}</td>
                    <td style={{ fontSize: 13, color: 'var(--text-muted)' }}>{r.workflow_name}</td>
                    <td><StatusBadge status={r.status} /></td>
                    <td style={{ fontSize: 13 }}>{formatTime(r.created_at)}</td>
                    <td style={{ fontSize: 13, color: 'var(--text-muted)' }}>
                      {r.finished_at ? formatDuration((new Date(r.finished_at).getTime() - new Date(r.created_at).getTime()) / 1000) : '—'}
                    </td>
                    <td onClick={(e) => e.stopPropagation()}>
                      <button className="btn btn-ghost btn-xs" style={{ color: 'var(--failed)' }}
                        onClick={() => setDeleteTarget(r.id)}>删除</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination page={store.page} total={store.total} pageSize={store.size} onChange={handlePageChange} />
        </>
      )}

      <ConfirmDialog
        open={!!deleteTarget}
        title="删除记录"
        message="确定要删除这条执行记录吗？此操作不可撤销。"
        confirmText="确认删除"
        onConfirm={() => { store.deleteSingle(deleteTarget!); setDeleteTarget(null); }}
        onCancel={() => setDeleteTarget(null)}
      />

      <ConfirmDialog
        open={showBatchDelete}
        title="批量删除"
        message={`确定要删除选中的 ${store.selectedIds.length} 条记录吗？此操作不可撤销。`}
        confirmText="确认删除"
        onConfirm={() => { store.batchDelete(); setShowBatchDelete(false); }}
        onCancel={() => setShowBatchDelete(false)}
      />
    </div>
  );
}
