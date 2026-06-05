import { useEffect, useState, useCallback, useRef } from 'react';
import { useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useWorkflowStore } from '../stores/workflowStore';
import { DagCanvas } from '../components/DagViewer/DagCanvas';
import { StepEditor } from '../components/DagViewer/StepEditor';
import { PluginCatalog } from '../components/PluginCatalog/PluginCatalog';
import { Modal } from '../components/ui/Modal';
import { LogPanel } from '../components/ui/LogPanel';
import { ProgressBar } from '../components/ui/ProgressBar';
import { Badge } from '../components/ui/Badge';
import { Spinner } from '../components/ui/Feedback';
import * as api from '../api';
import type { PluginInfo, StepRecord } from '../types';

const mapStatus = (state: string): 'pending' | 'running' | 'success' | 'failed' | 'skipped' => {
  if (state === 'success') return 'success';
  if (state === 'failed') return 'failed';
  if (state === 'running') return 'running';
  if (state === 'cancelled') return 'skipped';
  return 'pending';
};

export function WorkflowPage() {
  const { t } = useTranslation();
  const { project } = useParams<{ project: string }>();
  const store = useWorkflowStore();
  const sseRef = useRef<EventSource | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const [showNewWf, setShowNewWf] = useState(false);
  const [showClone, setShowClone] = useState(false);
  const [newWfName, setNewWfName] = useState('');
  const [cloneSrc, setCloneSrc] = useState('');
  const [cloneName, setCloneName] = useState('');
  const [workflows, setWorkflows] = useState<{ name: string; label: string }[]>([]);
  const [configText, setConfigText] = useState('');
  const [showResult, setShowResult] = useState(false);
  const [elapsed, setElapsed] = useState('0s');
  const [startTime, setStartTime] = useState<Date | null>(null);

  // Load project
  useEffect(() => {
    if (!project) return;
    store.load(project);
  }, [project]);

  // Cleanup SSE on unmount
  useEffect(() => {
    return () => {
      sseRef.current?.close();
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, []);

  // SSE connection for in-canvas execution
  useEffect(() => {
    if (store.resultStatus !== 'running' || !store.resultRunId) return;

    const runId = store.resultRunId;
    setStartTime(new Date());
    setShowResult(true);

    // Elapsed timer
    timerRef.current = setInterval(() => {
      setElapsed((prev) => {
        const match = prev.match(/^(\d+)/);
        if (match) return `${parseInt(match[1]) + 1}s`;
        return prev;
      });
    }, 1000);

    // Sync initial state — catch up on steps that finished before SSE connected
    let initialSyncDone = false;
    api.getRunDetail(runId).then((doc) => {
      useWorkflowStore.setState((s) => {
        const knownSteps = new Set(s.resultSteps.map((r) => r.step_name));
        const updatedNodes = s.dagNodes.map((n) => {
          if (n.status !== 'pending') return n;
          const ss = doc.steps.find((x) => x.step_name === n.id);
          if (!ss) return n;
          return { ...n, status: mapStatus(ss.state) };
        });
        const mergedSteps = s.resultSteps.map((r) => {
          const ss = doc.steps.find((x) => x.step_name === r.step_name);
          if (!ss || r.state !== 'running') return r;
          return { ...r, state: ss.state as StepRecord['state'], duration: ss.duration, error: ss.error || '' };
        });
        for (const ss of doc.steps) {
          if (!knownSteps.has(ss.step_name)) {
            mergedSteps.push({
              run_id: runId, step_name: ss.step_name,
              state: ss.state as StepRecord['state'],
              data: ss.data || '', error: ss.error || '',
              duration: ss.duration, target: ss.target || '',
            });
          }
        }
        return { dagNodes: updatedNodes, resultSteps: mergedSteps };
      });
      initialSyncDone = true;
    });

    // SSE connection
    const es = new EventSource(`/api/sse/${runId}`);
    sseRef.current = es;

    es.onmessage = (event) => {
      const raw = JSON.parse(event.data);
      const items = Array.isArray(raw) ? raw : [raw];

      for (const data of items) {
        if (data.type === 'step_start') {
          useWorkflowStore.setState((s) => {
            if (s.resultSteps.some((r) => r.step_name === data.step)) return s;
            const updatedNodes = s.dagNodes.map((n) => {
              if (n.id !== data.step || n.status !== 'pending') return n;
              return { ...n, status: 'running' as const };
            });
            const newEntry: StepRecord = {
              run_id: data.run_id, step_name: data.step,
              state: 'running', data: '', error: '', duration: 0, target: data.target || '',
            };
            return { dagNodes: updatedNodes, resultSteps: [...s.resultSteps, newEntry] };
          });
        } else if (data.type === 'step_end') {
          const st = mapStatus(data.state);
          useWorkflowStore.setState((s) => {
            const updatedNodes = s.dagNodes.map((n) => {
              if (n.id !== data.step || n.status !== 'running') return n;
              return { ...n, status: st };
            });
            const updatedSteps = s.resultSteps.map((r) => {
              if (r.step_name !== data.step || r.state !== 'running') return r;
              return { ...r, state: data.state as StepRecord['state'], duration: data.duration, error: data.error || '' };
            });
            return { dagNodes: updatedNodes, resultSteps: updatedSteps };
          });
        } else if (data.type === 'log') {
          useWorkflowStore.setState((s) => ({
            resultLogs: [...s.resultLogs, {
              run_id: data.run_id, step_name: data.step_name,
              timestamp: data.timestamp, level: 'INFO', message: data.message,
              _idx: Math.random().toString(36).slice(2),
            }].slice(-500),
          }));
        } else if (data.type === 'completed') {
          useWorkflowStore.setState({ resultStatus: data.status, running: false });
          es.close();
        }
      }
    };

    es.onerror = () => {
      es.close();
      const poll = setInterval(async () => {
        try {
          const doc = await api.getRunDetail(runId);
          const st = useWorkflowStore.getState();

          // Update DAG nodes: exact ID match
          const newNodes = [...st.dagNodes];
          for (const ss of doc.steps) {
            const mappedStatus = mapStatus(ss.state);
            for (let i = 0; i < newNodes.length; i++) {
              const n = newNodes[i];
              if (n.id === ss.step_name) {
                newNodes[i] = { ...n, status: mappedStatus };
                break;
              }
            }
          }

          // Update resultSteps: exact step_name match
          const remainingSteps = [...doc.steps];
          const newResultSteps = st.resultSteps.map((r) => {
            const idx = remainingSteps.findIndex((ss) => ss.step_name === r.step_name);
            if (idx === -1) return r;
            const [ss] = remainingSteps.splice(idx, 1);
            return { ...r, state: ss.state as StepRecord['state'], duration: ss.duration, error: ss.error || '' };
          });
          // Add any new steps not yet tracked
          for (const ss of remainingSteps) {
            newResultSteps.push({
              run_id: runId, step_name: ss.step_name,
              state: ss.state as StepRecord['state'], data: '', error: ss.error || '',
              duration: ss.duration, target: ss.target,
            });
          }

          useWorkflowStore.setState({
            dagNodes: newNodes,
            resultSteps: newResultSteps,
            resultLogs: doc.logs.map((l: { run_id: string; step_name: string; timestamp: string; message: string }, i: number) => ({
              run_id: l.run_id, step_name: l.step_name, timestamp: l.timestamp, level: 'INFO' as const, message: l.message,
              _idx: `poll-${i}`,
            })).slice(-500),
            resultStatus: doc.run.status,
            running: doc.run.status === 'running',
          });
          if (doc.run.status !== 'running') {
            clearInterval(poll);
          }
        } catch { clearInterval(poll); }
      }, 2000);
    };

    return () => {
      es.close();
    };
  }, [store.resultRunId, store.resultStatus]);

  const handleRun = async () => {
    setElapsed('0s');
    setStartTime(null);
    await store.run(false);
  };

  const handleCancel = async () => {
    sseRef.current?.close();
    if (timerRef.current) clearInterval(timerRef.current);
    await store.cancelRun();
  };

  const handleDryRun = async () => {
    await store.run(true);
    setShowResult(true);
  };

  const handleCreateWorkflow = async () => {
    if (!newWfName.trim()) return;
    try {
      await api.createWorkflow({ name: newWfName.trim() });
      setShowNewWf(false);
      setNewWfName('');
      store.load(project!);
    } catch { /* ignore */ }
  };

  const handleClone = async () => {
    if (!cloneName.trim() || !cloneSrc) return;
    try {
      await api.cloneWorkflow(cloneSrc, { name: cloneName.trim() });
      setShowClone(false);
      setCloneName('');
      store.load(project!);
    } catch { /* ignore */ }
  };

  const openClone = async () => {
    try {
      const wfs = await api.getWorkflows();
      setWorkflows(wfs);
    } catch { setWorkflows([]); }
    setShowClone(true);
  };

  const handleConfigEditor = async () => {
    setShowResult(false);
    try {
      const { config } = await api.getConfig();
      setConfigText(JSON.stringify(config, null, 2));
      store.setConfigEditor(true);
    } catch { /* ignore */ }
  };

  const handleSaveConfig = async () => {
    try {
      const config = JSON.parse(configText);
      await store.saveConfig(config);
      store.setConfigEditor(false);
    } catch {
      // JSON parse error
    }
  };

  const handleAddNodeFromPort = useCallback(async (fromNodeId: string, plugin: PluginInfo) => {
    await store.addStep(plugin);
    const state = useWorkflowStore.getState();
    const existingTargets = new Set(state.dagEdges.filter((e) => e.from === fromNodeId).map((e) => e.to));
    const newNode = state.dagNodes.find((n) =>
      n.plugin === plugin.name && !existingTargets.has(n.id) && n.id !== fromNodeId,
    );
    if (newNode) {
      store.addEdge(fromNodeId, newNode.id);
    }
  }, [store]);

  const completedSteps = store.resultSteps.filter((s) => s.state === 'success').length;
  const failedSteps = store.resultSteps.filter((s) => s.state === 'failed').length;

  if (store.loading) {
    return <div className="empty" style={{ padding: 80 }}><Spinner /><p style={{ marginTop: 12 }}>{t('common.loading')}</p></div>;
  }

  const isExecuting = store.running;

  return (
    <div>
      {/* Breadcrumb + Toolbar */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
        <div className="breadcrumb" style={{ marginBottom: 0 }}>
          <a href="/projects">{t('page.projects')}</a>
          <span className="sep">/</span>
          <span>{project}</span>
          <span className="sep">/</span>
          <span>{t('wf.editor')}</span>
          <span style={{ fontSize: 12, color: 'var(--text-muted)', marginLeft: 8 }}>
            {store.workflowLabel} ({store.dagNodes.length} {t('wf.stepsCount')})
          </span>
          {isExecuting && (
            <span style={{ marginLeft: 12 }}>
              <Badge type="running">{t('monitor.elapsed')} {elapsed}</Badge>
            </span>
          )}
        </div>
        <div className="btn-group">
          <button className="btn btn-outline btn-sm" onClick={() => setShowNewWf(true)} disabled={isExecuting}>
            {t('wf.new')}
          </button>
          <button className="btn btn-outline btn-sm" onClick={openClone} disabled={isExecuting}>
            {t('wf.clone')}
          </button>
          <button className="btn btn-outline btn-sm" onClick={handleConfigEditor} disabled={isExecuting}>
            {t('wf.jsonConfig')}
          </button>
          <button className="btn btn-outline btn-sm" onClick={handleDryRun} disabled={isExecuting}>
            {t('wf.preview')}
          </button>
          {isExecuting ? (
            <button className="btn btn-sm" style={{ background: 'var(--failed)', color: '#fff' }} onClick={handleCancel}>
              ■ {t('monitor.cancel')}
            </button>
          ) : (
            <button className="btn btn-primary btn-sm" onClick={handleRun} disabled={store.loading}>
              ▶ {t('wf.execute')}
            </button>
          )}
        </div>
      </div>

      {/* Main editor area */}
      {store.showConfigEditor ? (
        <div className="card" style={{ height: 'calc(100vh - 180px)' }}>
          <div className="card-header">
            <h4>{t('wf.jsonConfig')}</h4>
            <div className="btn-group">
              <button className="btn btn-outline btn-sm"
                onClick={() => store.setConfigEditor(false)}>{t('common.cancel')}</button>
              <button className="btn btn-primary btn-sm" onClick={handleSaveConfig}>
                {t('wf.save')}
              </button>
            </div>
          </div>
          <div className="card-body" style={{ padding: 0, height: 'calc(100% - 52px)' }}>
            <textarea
              className="form-input"
              value={configText}
              onChange={(e) => setConfigText(e.target.value)}
              style={{ width: '100%', height: '100%', border: 'none', borderRadius: 0, resize: 'none', padding: 16 }}
            />
          </div>
        </div>
      ) : (
        <>
          {/* Execution progress bar */}
          {isExecuting && store.resultSteps.length > 0 && (
            <div style={{ marginBottom: 8 }}>
              <ProgressBar
                value={completedSteps}
                max={store.resultSteps.length}
                state={failedSteps > 0 ? 'failed' : 'success'}
              />
            </div>
          )}

          <div className="card" style={{
            display: 'flex',
            height: isExecuting ? 'calc(100vh - 480px)' : 'calc(100vh - 200px)',
            overflow: 'hidden',
          }}>
            {/* Left: Plugin Catalog */}
            <PluginCatalog />

            {/* Center: DAG Canvas */}
            <DagCanvas
              nodes={store.dagNodes}
              edges={store.dagEdges}
              selectedNodeId={store.selectedNodeId}
              onNodeClick={(id) => !isExecuting && store.selectNode(id)}
              onEdgeCreate={(from, to) => store.addEdge(from, to)}
              onEdgeDelete={(from, to) => store.removeEdge(from, to)}
              onAddNodeFromPort={handleAddNodeFromPort}
              plugins={[...store.sharedPlugins, ...store.projectPlugins]}
              onAutoLayout={() => store.resetLayout()}
              readonly={isExecuting}
            />

            {/* Right: Properties Panel */}
            {!isExecuting && <StepEditor />}
          </div>

          {/* Execution result panel */}
          {showResult && (
            <>
              {/* Step results */}
              {store.resultSteps.length > 0 && (
                <div className="card" style={{ marginTop: 12 }}>
                  <div className="card-header">
                    <h4>
                      {t('monitor.steps')}
                      <span style={{ fontWeight: 400, fontSize: 12, color: 'var(--text-muted)', marginLeft: 8 }}>
                        {completedSteps}/{store.resultSteps.length} {failedSteps > 0 ? `· ${failedSteps} 失败` : ''}
                      </span>
                    </h4>
                    <div className="btn-group">
                      <Badge type={store.resultStatus === 'success' ? 'success' : store.resultStatus === 'failed' ? 'failed' : store.resultStatus === 'running' ? 'running' : 'pending'}>
                        {store.resultStatus || '—'}
                      </Badge>
                      <button className="btn btn-ghost btn-xs" onClick={() => { setShowResult(false); store.load(project!); }}>
                        ✕
                      </button>
                    </div>
                  </div>
                  <div className="card-body" style={{ padding: 0 }}>
                    {store.resultSteps.map((s, i) => {
                      const dur = s.duration > 0 ? `${s.duration.toFixed(0)}s` : '';
                      return (
                        <div key={`${s.step_name}-${i}`} style={{
                          display: 'flex', alignItems: 'center', gap: 10,
                          padding: '6px 14px', borderBottom: '1px solid var(--border)',
                        }}>
                          <span style={{
                            display: 'inline-block', width: 20, height: 20, borderRadius: '50%',
                            textAlign: 'center', lineHeight: '20px', color: '#fff', fontSize: 10, flexShrink: 0,
                            background: s.state === 'success' ? 'var(--success)'
                              : s.state === 'failed' ? 'var(--failed)'
                              : s.state === 'running' ? 'var(--running)'
                              : 'var(--pending)',
                          }}>
                            {s.state === 'success' ? '✓' : s.state === 'failed' ? '✗' : s.state === 'running' ? '●' : '●'}
                          </span>
                          <span style={{ flex: 1, fontSize: 13, fontWeight: 500 }}>{s.step_name}</span>
                          <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{s.target} {dur}</span>
                          <Badge type={s.state === 'success' ? 'success' : s.state === 'failed' ? 'failed' : s.state === 'running' ? 'running' : 'pending'}>
                            {s.state}
                          </Badge>
                        </div>
                      );
                    })}
                  </div>
                </div>
              )}

              {/* Dry run plan */}
              {store.resultPlan.length > 0 && (
                <div className="card" style={{ marginTop: 12 }}>
                  <div className="card-header">
                    <h4>{t('wf.plan')}</h4>
                    <button className="btn btn-ghost btn-xs" onClick={() => setShowResult(false)}>✕</button>
                  </div>
                  <div className="card-body">
                    <table>
                      <thead>
                        <tr>
                          <th>#</th><th>Plugin</th><th>Target</th><th>Dependencies</th>
                        </tr>
                      </thead>
                      <tbody>
                        {store.resultPlan.map((p) => (
                          <tr key={p.plugin}>
                            <td>{p.index}</td>
                            <td><strong>{p.plugin}</strong></td>
                            <td>{p.target}</td>
                            <td>{(p.depends_on || []).join(', ') || '—'}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}

              {/* Log panel */}
              {store.resultLogs.length > 0 && (
                <div className="card" style={{ marginTop: 12 }}>
                  <div className="card-header">
                    <h4>日志 ({store.resultLogs.length})</h4>
                  </div>
                  <div className="card-body" style={{ padding: 0 }}>
                    <LogPanel logs={store.resultLogs} maxHeight={250} />
                  </div>
                </div>
              )}
            </>
          )}
        </>
      )}

      {/* New Workflow Modal */}
      <Modal
        open={showNewWf}
        title={t('wf.newWorkflow')}
        onClose={() => setShowNewWf(false)}
        footer={
          <>
            <button className="btn btn-outline btn-sm" onClick={() => setShowNewWf(false)}>{t('common.cancel')}</button>
            <button className="btn btn-primary btn-sm" onClick={handleCreateWorkflow}>{t('wf.create')}</button>
          </>
        }
      >
        <div className="form-group">
          <label>{t('wf.workflowName')}</label>
          <input className="form-input" value={newWfName} onChange={(e) => setNewWfName(e.target.value)}
            placeholder="my_workflow" autoFocus />
        </div>
      </Modal>

      {/* Clone Modal */}
      <Modal
        open={showClone}
        title={t('wf.clone')}
        onClose={() => setShowClone(false)}
        footer={
          <>
            <button className="btn btn-outline btn-sm" onClick={() => setShowClone(false)}>{t('common.cancel')}</button>
            <button className="btn btn-primary btn-sm" onClick={handleClone}>{t('wf.confirmClone')}</button>
          </>
        }
      >
        <div className="form-group">
          <label>{t('wf.sourceWorkflow')}</label>
          <select className="form-input" value={cloneSrc} onChange={(e) => setCloneSrc(e.target.value)}>
            <option value="">选择工作流</option>
            {workflows.map((w) => (
              <option key={w.name} value={w.name}>{w.label || w.name}</option>
            ))}
          </select>
        </div>
        <div className="form-group">
          <label>{t('wf.newWorkflowName')}</label>
          <input className="form-input" value={cloneName} onChange={(e) => setCloneName(e.target.value)}
            placeholder="workflow_copy" />
        </div>
      </Modal>
    </div>
  );
}
