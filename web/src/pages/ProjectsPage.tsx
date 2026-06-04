import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useProjectStore } from '../stores/projectStore';
import { Modal, ConfirmDialog } from '../components/ui/Modal';
import { Badge } from '../components/ui/Badge';
import { EmptyState, Spinner } from '../components/ui/Feedback';
import { Pagination } from '../components/ui/Pagination';
import { runProject } from '../api/execution';

export function ProjectsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const store = useProjectStore();

  const [showCreate, setShowCreate] = useState(false);
  const [showDelete, setShowDelete] = useState<string | null>(null);
  const [newName, setNewName] = useState('');
  const [newId, setNewId] = useState('');
  const [newWorkflow, setNewWorkflow] = useState('');
  const [preview, setPreview] = useState<{ project: string; data: unknown } | null>(null);

  useEffect(() => {
    store.load();
  }, []);

  const filtered = store.projects.filter((p) => {
    const s = store.search.toLowerCase();
    return !s || p.name.toLowerCase().includes(s) || p.project_id.toLowerCase().includes(s);
  });

  const paged = filtered.slice((store.page - 1) * store.pageSize, store.page * store.pageSize);
  const totalPages = Math.max(1, Math.ceil(filtered.length / store.pageSize));

  const handleCreate = async () => {
    if (!newName.trim()) return;
    const ok = await store.create(newName.trim(), newId.trim(), newWorkflow.trim());
    if (ok) {
      setShowCreate(false);
      setNewName('');
      setNewId('');
      setNewWorkflow('');
    }
  };

  const handleDelete = async (name: string) => {
    await store.deleteProject(name);
    setShowDelete(null);
  };

  const handlePreview = async (projectName: string) => {
    try {
      const result = await runProject(projectName, true);
      setPreview({ project: projectName, data: result });
    } catch {
      // ignore
    }
  };

  const handleRun = (projectName: string) => {
    navigate(`/run/${encodeURIComponent(projectName)}`);
  };

  const getStatusBadge = (status?: string) => {
    if (!status) return <Badge type="pending">—</Badge>;
    const map: Record<string, 'success' | 'failed' | 'running' | 'skipped'> = {
      success: 'success', failed: 'failed', running: 'running', cancelled: 'skipped',
    };
    return <Badge type={map[status] || 'pending'}>{status}</Badge>;
  };

  if (store.loading && store.projects.length === 0) {
    return (
      <div className="empty" style={{ padding: 80 }}>
        <Spinner />
        <p style={{ marginTop: 12 }}>{t('common.loading')}</p>
      </div>
    );
  }

  return (
    <div>
      {/* Stats */}
      <div className="grid-4" style={{ marginBottom: 20 }}>
        <div className="card stat-card">
          <div className="stat-value">{store.stats.total}</div>
          <div className="stat-label">{t('project.total')}</div>
        </div>
        <div className="card stat-card">
          <div className="stat-value" style={{ color: 'var(--success)' }}>{store.stats.enabled}</div>
          <div className="stat-label">{t('project.enabled')}</div>
        </div>
        <div className="card stat-card">
          <div className="stat-value" style={{ color: 'var(--text-muted)' }}>{store.stats.disabled}</div>
          <div className="stat-label">{t('project.disabled')}</div>
        </div>
        <div className="card stat-card">
          <div className="stat-value" style={{ color: 'var(--running)' }}>{store.stats.runs24h}</div>
          <div className="stat-label">{t('project.recent24h')}</div>
        </div>
      </div>

      {/* Toolbar */}
      <div style={{ display: 'flex', gap: 12, marginBottom: 16, flexWrap: 'wrap' }}>
        <div className="search-box" style={{ flex: '1 1 200px' }}>
          <span className="search-icon">🔍</span>
          <input
            className="form-input"
            placeholder={t('project.search')}
            value={store.search}
            onChange={(e) => store.setSearch(e.target.value)}
          />
        </div>
        <button className="btn btn-outline btn-sm" onClick={() => store.load()}>
          {t('project.refresh')}
        </button>
        <button className="btn btn-primary btn-sm" onClick={() => setShowCreate(true)}>
          {t('project.create')}
        </button>
      </div>

      {/* Project Grid */}
      {filtered.length === 0 ? (
        <EmptyState message={store.search ? t('project.noMatch') : t('project.noData')} />
      ) : (
        <>
          <div className="grid-3">
            {paged.map((p) => (
              <div key={p.name} className="card fade-in">
                <div className="card-body" style={{ padding: 16 }}>
                  <div style={{ display: 'flex', alignItems: 'start', gap: 12, marginBottom: 8 }}>
                    <div style={{
                      width: 40, height: 40, borderRadius: 8,
                      background: p.enabled ? 'var(--primary-light)' : 'var(--pending-light)',
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                      fontWeight: 700, fontSize: 16,
                      color: p.enabled ? 'var(--primary)' : 'var(--text-muted)',
                    }}>
                      {p.name[0]?.toUpperCase()}
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontWeight: 600, fontSize: 15, marginBottom: 2 }}>{p.name}</div>
                      <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                        {p.project_id} · {p.workflow}
                      </div>
                    </div>
                    <label style={{ display: 'flex', alignItems: 'center', gap: 4, cursor: 'pointer' }}>
                      <input
                        type="checkbox"
                        checked={p.enabled}
                        onChange={() => store.toggleEnabled(p.name, !p.enabled)}
                        style={{ accentColor: 'var(--primary)' }}
                      />
                    </label>
                  </div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
                    <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>上次:</span>
                    {getStatusBadge(p.last_run?.status)}
                    <span style={{ fontSize: 11, color: 'var(--text-muted)', marginLeft: 'auto' }}>
                      {p.last_run?.created_at ? new Date(p.last_run.created_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '—'}
                    </span>
                  </div>
                  <div className="btn-group" style={{ flexWrap: 'wrap' }}>
                    <button className="btn btn-primary btn-xs" onClick={() => handleRun(p.name)}>
                      ▶ {t('project.execute')}
                    </button>
                    <button className="btn btn-outline btn-xs" onClick={() => handlePreview(p.name)}>
                      {t('project.preview')}
                    </button>
                    <button className="btn btn-ghost btn-xs" onClick={() => navigate(`/workflow/${encodeURIComponent(p.name)}`)}>
                      {t('project.detail')}
                    </button>
                    <button className="btn btn-ghost btn-xs" style={{ color: 'var(--failed)', marginLeft: 'auto' }}
                      onClick={() => setShowDelete(p.name)}>
                      删除
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
          <Pagination page={store.page} total={filtered.length} pageSize={store.pageSize} onChange={store.setPage} />
        </>
      )}

      {/* Create Modal */}
      <Modal
        open={showCreate}
        title={t('project.create')}
        onClose={() => setShowCreate(false)}
        footer={
          <>
            <button className="btn btn-outline btn-sm" onClick={() => setShowCreate(false)}>{t('common.cancel')}</button>
            <button className="btn btn-primary btn-sm" onClick={handleCreate} disabled={!newName.trim()}>
              {t('project.create')}
            </button>
          </>
        }
      >
        <div className="form-group">
          <label>{t('project.name')}</label>
          <input className="form-input" value={newName} onChange={(e) => setNewName(e.target.value)}
            placeholder="my_project" autoFocus />
        </div>
        <div className="form-group">
          <label>{t('project.id')}</label>
          <input className="form-input" value={newId} onChange={(e) => setNewId(e.target.value)}
            placeholder="001" />
        </div>
        <div className="form-group">
          <label>{t('project.workflow')}</label>
          <input className="form-input" value={newWorkflow} onChange={(e) => setNewWorkflow(e.target.value)}
            placeholder="standard" />
        </div>
      </Modal>

      {/* Delete Confirm */}
      <ConfirmDialog
        open={!!showDelete}
        title={t('project.delete')}
        message={`${t('project.deleteConfirm')} "${showDelete}"? ${t('project.deleteWarning')}`}
        confirmText={t('project.confirmDelete')}
        onConfirm={() => showDelete && handleDelete(showDelete)}
        onCancel={() => setShowDelete(null)}
      />

      {/* Preview Modal */}
      <Modal
        open={!!preview}
        title={`执行计划: ${preview?.project || ''}`}
        onClose={() => setPreview(null)}
        width={700}
      >
        <pre style={{
          background: '#1A1A2E', color: '#CDD6F4', padding: 16, borderRadius: 8,
          fontSize: 12, overflow: 'auto', maxHeight: 400
        }}>
          {JSON.stringify(preview?.data, null, 2)}
        </pre>
      </Modal>
    </div>
  );
}
