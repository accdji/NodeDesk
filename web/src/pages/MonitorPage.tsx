import { useEffect, useRef, useState, useCallback } from 'react';
import { useParams, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { StepRecord, LogEntry } from '../types';
import * as api from '../api';
import { StatusBadge } from '../components/ui/Badge';
import { LogPanel } from '../components/ui/LogPanel';
import { ProgressBar } from '../components/ui/ProgressBar';
import { Spinner } from '../components/ui/Feedback';
import { formatDuration, STATUS_COLORS, STATUS_ICONS } from '../utils/constants';

export function MonitorPage() {
  const { t } = useTranslation();
  const { project } = useParams<{ project: string }>();
  const [searchParams] = useSearchParams();
  const rid = searchParams.get('rid') || '';

  const [runId, setRunId] = useState(rid);
  const [status, setStatus] = useState<string>('running');
  const [steps, setSteps] = useState<StepRecord[]>([]);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [projectName, setProjectName] = useState(project || '');
  const [startTime, setStartTime] = useState<Date | null>(null);
  const [elapsed, setElapsed] = useState('0s');
  const logRef = useRef<HTMLDivElement>(null);

  // Auto scroll log
  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [logs]);

  // Elapsed timer
  useEffect(() => {
    if (!startTime || status === 'success' || status === 'failed' || status === 'cancelled') return;
    const timer = setInterval(() => {
      const s = (Date.now() - startTime.getTime()) / 1000;
      setElapsed(formatDuration(s));
    }, 500);
    return () => clearInterval(timer);
  }, [startTime, status]);

  // Load initial state or start run
  useEffect(() => {
    if (!project) return;

    if (rid) {
      // Load existing run
      setRunId(rid);
      api.getRunDetail(rid).then((doc) => {
        setProjectName(doc.run.project_name);
        setStatus(doc.run.status);
        setSteps(doc.steps);
        setLogs(doc.logs.map((l, i) => ({ ...l, _idx: `init-${i}` })));
        setStartTime(new Date(doc.run.created_at));
      });
    } else {
      // Start new run
      api.runProject(project).then((result) => {
        if ('run_id' in result) {
          setRunId(result.run_id);
          setStartTime(new Date());
        }
      });
    }
  }, [project, rid]);

  // SSE connection
  useEffect(() => {
    if (!runId || (status !== 'running' && status !== 'pending')) return;

    const es = new EventSource(`/api/sse/${runId}`);
    es.onmessage = (event) => {
      const raw = JSON.parse(event.data);
      const items = Array.isArray(raw) ? raw : [raw];

      for (const data of items) {
        if (data.type === 'step_start') {
          setSteps((prev) => [...prev, {
            run_id: data.run_id, step_name: data.step,
            state: 'running', data: '', error: '', duration: 0, target: data.target || '',
          }]);
        } else if (data.type === 'step_end') {
          setSteps((prev) => {
            let foundRunning = false;
            return prev.map((s) => {
              if (s.step_name !== data.step) return s;
              if (!foundRunning && s.state === 'running') {
                foundRunning = true;
                return { ...s, state: data.state, duration: data.duration, error: data.error || '' };
              }
              return s;
            });
          });
        } else if (data.type === 'log') {
          setLogs((prev) => {
            const entry: LogEntry = {
              run_id: data.run_id, step_name: data.step_name,
              timestamp: data.timestamp, level: 'INFO', message: data.message,
              _idx: Math.random().toString(36).slice(2),
            };
            const next = [...prev, entry];
            return next.length > 800 ? next.slice(-500) : next;
          });
        } else if (data.type === 'completed') {
          setStatus(data.status);
          es.close();
        }
      }
    };

    es.onerror = () => {
      // SSE error - poll as fallback
      es.close();
      const poll = setInterval(async () => {
        try {
          const doc = await api.getRunDetail(runId);
          setSteps(doc.steps);
          setLogs(doc.logs.map((l, i) => ({ ...l, _idx: `poll-${i}` })));
          setStatus(doc.run.status);
          if (doc.run.status !== 'running') {
            clearInterval(poll);
          }
        } catch { clearInterval(poll); }
      }, 2000);
    };

    return () => {
      es.close();
    };
  }, [runId, status]);

  const handleCancel = useCallback(async () => {
    if (!runId) return;
    await api.cancelRun(runId);
  }, [runId]);

  const completedSteps = steps.filter((s) => s.state === 'success').length;
  const failedSteps = steps.filter((s) => s.state === 'failed').length;

  if (!runId) {
    return (
      <div>
        <div className="breadcrumb">
          <a href="/projects">{t('page.projects')}</a>
          <span className="sep">/</span>
          <span>{project}</span>
        </div>
        <div className="empty" style={{ padding: 80 }}>
          <Spinner />
          <p style={{ marginTop: 12 }}>{t('monitor.waiting')}</p>
          <p style={{ color: 'var(--text-muted)', fontSize: 13 }}>{t('monitor.waitDesc')}</p>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className="breadcrumb">
        <a href="/projects">{t('page.projects')}</a>
        <span className="sep">/</span>
        <span>{projectName}</span>
        <span className="sep">/</span>
        <span>{t('monitor.title')}</span>
      </div>

      {/* Status Bar */}
      <div className="card" style={{ marginBottom: 16, padding: '14px 18px', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <div style={{
            width: 36, height: 36, borderRadius: '50%',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            fontSize: 18, flexShrink: 0,
            background: STATUS_COLORS[status] || '#94A3B8', color: '#fff', opacity: 0.9,
          }}>
            {STATUS_ICONS[status] || '●'}
          </div>
          <div>
            <span style={{ fontWeight: 700, fontSize: 16 }}>{projectName}</span>
            <StatusBadge status={status} />
            <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 2 }}>
              {t('monitor.elapsed')} {elapsed} · {t('monitor.steps')} {completedSteps}/{steps.length}
              · <code>#{runId.slice(0, 12)}</code>
            </div>
          </div>
        </div>
        <div>
          {status === 'running' ? (
            <button className="btn btn-outline btn-sm" onClick={handleCancel}>{t('monitor.cancel')}</button>
          ) : (
            <a href={`/history/${runId}`} className="btn btn-ghost btn-sm">{t('monitor.viewDetail')}</a>
          )}
        </div>
      </div>

      {/* Progress */}
      {steps.length > 0 && (
        <div style={{ marginBottom: 16 }}>
          <ProgressBar value={completedSteps} max={steps.length} state={failedSteps > 0 ? 'failed' : 'success'} />
        </div>
      )}

      {/* DAG SVG */}
      {steps.length > 0 && <DagMonitorSVG steps={steps} />}

      {/* Steps */}
      {steps.length > 0 && (
        <div className="card" style={{ marginBottom: 16 }}>
          <div className="card-header"><h4>{t('monitor.steps')}</h4></div>
          <div className="card-body" style={{ padding: 0 }}>
            {steps.map((s, i) => {
              const dur = s.duration > 0 ? formatDuration(s.duration) : '';
              return (
                <div key={`${s.step_name}-${i}`} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 14px', borderBottom: '1px solid var(--border)' }}>
                  <span style={{
                    display: 'inline-block', width: 22, height: 22, borderRadius: '50%',
                    background: STATUS_COLORS[s.state] || '#94A3B8',
                    textAlign: 'center', lineHeight: '22px', color: '#fff', fontSize: 10, flexShrink: 0,
                  }}>
                    {STATUS_ICONS[s.state] || '●'}
                  </span>
                  <span style={{ flex: 1, fontSize: 13, fontWeight: 500 }}>{s.step_name}</span>
                  <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{s.target} {dur}</span>
                  <StatusBadge status={s.state} />
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Logs */}
      <div className="card">
        <div className="card-header"><h4>{t('monitor.filterLog')}</h4></div>
        <div className="card-body" style={{ padding: 0 }}>
          <div ref={logRef}>
            <LogPanel logs={logs} maxHeight={500} />
          </div>
        </div>
      </div>
    </div>
  );
}

function DagMonitorSVG({ steps }: { steps: StepRecord[] }) {
  const n = steps.length;
  if (n === 0) return null;

  const nw = 150, nh = 38;
  const marginX = 50, marginY = 40;
  const spacing = 190;
  const w = marginX * 2 + (n - 1) * spacing + nw;
  const h = marginY * 2 + nh;

  const buildPath = (x1: number, y1: number, x2: number, y2: number) => {
    const dx = Math.abs(x2 - x1);
    const cp = Math.min(dx * 0.5, 100);
    return `M ${x1} ${y1} C ${x1 + cp} ${y1}, ${x2 - cp} ${y2}, ${x2} ${y2}`;
  };

  return (
    <div className="card" style={{ marginBottom: 16 }}>
      <div className="card-header"><h4>任务流程</h4></div>
      <div className="card-body" style={{ padding: 16, overflowX: 'auto' }}>
        <svg width={w} height={h} style={{ display: 'block', margin: '0 auto' }}>
          <defs>
            <marker id="arw" viewBox="0 0 10 10" refX={9} refY={5} markerWidth={6} markerHeight={6} orient="auto">
              <path d="M 0 1 L 8 5 L 0 9 Q 3 5 0 1 Z" fill="#94A3B8" />
            </marker>
            <filter id="ms">
              <feDropShadow dx={0} dy={1} stdDeviation={2} floodOpacity={0.08} />
            </filter>
          </defs>
          {Array.from({ length: n - 1 }, (_, i) => {
            const x1 = marginX + i * spacing + nw;
            const y1 = marginY + nh / 2;
            const x2 = marginX + (i + 1) * spacing;
            const y2 = marginY + nh / 2;
            return (
              <path key={i}
                d={buildPath(x1, y1, x2, y2)}
                fill="none" stroke="#94A3B8" strokeWidth={2} markerEnd="url(#arw)"
              />
            );
          })}
          {steps.map((s, i) => (
            <g key={s.step_name}>
              <rect x={marginX + i * spacing} y={marginY} width={nw} height={nh} rx={8}
                fill="#fff" stroke={STATUS_COLORS[s.state] || '#94A3B8'} strokeWidth={1.5}
                filter="url(#ms)" />
              <rect x={marginX + i * spacing} y={marginY} width={4} height={nh} rx={2}
                fill={STATUS_COLORS[s.state] || '#94A3B8'} />
              <text x={marginX + i * spacing + 16} y={marginY + 24} textAnchor="start"
                fill="#1E293B" fontSize={12} fontWeight={600}>{s.step_name}</text>
              <text x={marginX + i * spacing + nw - 10} y={marginY + 24} textAnchor="end"
                fill="#94A3B8" fontSize={10}>
                {s.state}{s.duration > 0 ? ` · ${formatDuration(s.duration)}` : ''}
              </text>
            </g>
          ))}
        </svg>
      </div>
    </div>
  );
}
