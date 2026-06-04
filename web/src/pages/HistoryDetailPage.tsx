import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { RunDoc } from '../types';
import * as api from '../api';
import { StatusBadge } from '../components/ui/Badge';
import { LogPanel } from '../components/ui/LogPanel';
import { EmptyState, Spinner } from '../components/ui/Feedback';
import { formatDuration, formatTime, STEP_BADGE_CLASS, STATUS_ICONS } from '../utils/constants';

export function HistoryDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [doc, setDoc] = useState<RunDoc | null>(null);
  const [loading, setLoading] = useState(true);
  const [logFilter, setLogFilter] = useState('');

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    api.getRunDetail(id).then(setDoc).catch(() => {}).finally(() => setLoading(false));
  }, [id]);

  if (loading) {
    return <div className="empty" style={{ padding: 80 }}><Spinner /><p style={{ marginTop: 12 }}>{t('common.loading')}</p></div>;
  }

  if (!doc) {
    return <EmptyState icon="🔍" message={t('common.error')} />;
  }

  const { run, steps, logs } = doc;
  const elapsed = run.finished_at
    ? formatDuration((new Date(run.finished_at).getTime() - new Date(run.created_at).getTime()) / 1000)
    : formatDuration(0);

  const filteredLogs = logFilter
    ? logs.filter((l) => l.message.toLowerCase().includes(logFilter.toLowerCase()) || l.step_name.toLowerCase().includes(logFilter.toLowerCase()))
    : logs;

  const maxDuration = Math.max(...steps.map((s) => s.duration), 0.01);

  return (
    <div>
      {/* Breadcrumb */}
      <div className="breadcrumb">
        <a href="/history">{t('page.history')}</a>
        <span className="sep">/</span>
        <span>{id?.slice(0, 16)}</span>
      </div>

      {/* Summary */}
      <div className="card">
        <div className="card-header">
          <h4>{t('detail.title')}</h4>
          <div className="btn-group">
            <button className="btn btn-outline btn-sm" onClick={() => id && api.runProject(run.project_name, false)}
              title={t('detail.rerun')}>🔄 {t('detail.rerun')}</button>
            <button className="btn btn-outline btn-sm"
              onClick={() => navigate(`/run/${encodeURIComponent(run.project_name)}?rid=${run.id}&from=failed`, {})}
              title={t('detail.rerunFailed')}>⏭ {t('detail.rerunFailed')}</button>
            <button className="btn btn-ghost btn-sm"
              onClick={() => navigate(`/workflow/${encodeURIComponent(run.project_name)}`)}>
              {t('detail.editWorkflow')}
            </button>
            <button className="btn btn-ghost btn-sm" onClick={() => api.downloadLog(run.id)}>
              📥 {t('detail.downloadLog')}
            </button>
          </div>
        </div>
        <div className="card-body">
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))', gap: 16 }}>
            <div>
              <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 2 }}>{t('project.name')}</div>
              <div style={{ fontWeight: 600 }}>{run.project_name}</div>
            </div>
            <div>
              <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 2 }}>{t('project.workflow')}</div>
              <div style={{ fontWeight: 600 }}>{run.workflow_name}</div>
            </div>
            <div>
              <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 2 }}>开始时间</div>
              <div style={{ fontWeight: 600 }}>{formatTime(run.created_at)}</div>
            </div>
            <div>
              <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 2 }}>状态</div>
              <StatusBadge status={run.status} />
            </div>
            <div>
              <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 2 }}>总耗时</div>
              <div style={{ fontWeight: 600 }}>{elapsed}</div>
            </div>
            <div>
              <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 2 }}>步骤数</div>
              <div style={{ fontWeight: 600 }}>{steps.length}</div>
            </div>
          </div>
        </div>
      </div>

      {/* Steps */}
      {steps.length > 0 && (
        <div className="card">
          <div className="card-header"><h4>步骤结果</h4></div>
          <div className="card-body" style={{ padding: 0 }}>
            {steps.map((s) => (
              <div key={s.step_name} style={{ padding: '10px 16px', borderBottom: '1px solid var(--border)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                  <div className={`step-icon ${s.state}`}>
                    {STATUS_ICONS[s.state] || '●'}
                  </div>
                  <div className="step-info">
                    <div className="step-name">{s.step_name}</div>
                    <div className="step-target">{s.target}</div>
                  </div>
                  <span style={{ fontSize: 13, color: 'var(--text-muted)' }}>
                    {s.duration > 0 ? formatDuration(s.duration) : '—'}
                  </span>
                  <span className={`badge ${STEP_BADGE_CLASS[s.state] || 'badge-pending'}`}>
                    {s.state}
                  </span>
                </div>
                {s.error && (
                  <div style={{ marginTop: 8, padding: 8, background: 'var(--failed-light)', borderRadius: 6, fontSize: 12, color: 'var(--failed)' }}>
                    {s.error}
                  </div>
                )}
                {s.data && (
                  <details style={{ marginTop: 8 }}>
                    <summary style={{ fontSize: 12, color: 'var(--primary)', cursor: 'pointer' }}>输出数据</summary>
                    <pre style={{ background: '#F8FAFC', padding: 8, borderRadius: 4, fontSize: 11, overflow: 'auto', maxHeight: 120, marginTop: 4 }}>
                      {(() => { try { return JSON.stringify(JSON.parse(s.data), null, 2); } catch { return s.data; } })()}
                    </pre>
                  </details>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Timing chart */}
      {steps.length > 0 && maxDuration > 0 && (
        <div className="card">
          <div className="card-header"><h4>{t('detail.duration')}</h4></div>
          <div className="card-body">
            {steps.map((s) => {
              const pct = maxDuration > 0 ? (s.duration / maxDuration) * 100 : 0;
              return (
                <div key={s.step_name} style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8 }}>
                  <span style={{ width: 120, fontSize: 13, textAlign: 'right', flexShrink: 0 }}>{s.step_name}</span>
                  <div style={{ flex: 1, height: 20, background: 'var(--border)', borderRadius: 4, overflow: 'hidden' }}>
                    <div style={{
                      height: '100%', width: `${Math.max(pct, 2)}%`, borderRadius: 4,
                      background: s.state === 'success' ? 'var(--success)' : s.state === 'failed' ? 'var(--failed)' : s.state === 'running' ? 'var(--running)' : 'var(--pending)',
                      transition: 'width 0.4s',
                    }} />
                  </div>
                  <span style={{ width: 50, fontSize: 12, color: 'var(--text-muted)' }}>{formatDuration(s.duration)}</span>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Logs */}
      <div className="card">
        <div className="card-header">
          <h4>执行日志 ({logs.length})</h4>
          <input className="form-input" style={{ width: 200, fontSize: 12 }}
            placeholder="过滤关键词..." value={logFilter}
            onChange={(e) => setLogFilter(e.target.value)} />
        </div>
        <div className="card-body" style={{ padding: 0 }}>
          <LogPanel logs={filteredLogs} />
        </div>
      </div>
    </div>
  );
}
