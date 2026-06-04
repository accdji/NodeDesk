import type { DagNode, ParamDef } from '../../types';
import { useWorkflowStore } from '../../stores/workflowStore';
import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '../ui/Modal';

const NODE_TYPES = [
  { value: 'script', label: 'step.typeScript' },
  { value: 'condition', label: 'step.typeCondition' },
  { value: 'loop', label: 'step.typeLoop' },
  { value: 'start', label: 'step.typeStart' },
  { value: 'end', label: 'step.typeEnd' },
];

export function StepEditor() {
  const { t } = useTranslation();
  const selectedNodeId = useWorkflowStore((s) => s.selectedNodeId);
  const nodes = useWorkflowStore((s) => s.dagNodes);
  const updateNode = useWorkflowStore((s) => s.updateNode);
  const saveStep = useWorkflowStore((s) => s.saveStep);
  const deleteStep = useWorkflowStore((s) => s.deleteStep);
  const selectNode = useWorkflowStore((s) => s.selectNode);

  const node = nodes.find((n) => n.id === selectedNodeId);

  if (!node) {
    return (
      <div style={{ width: 340, flexShrink: 0, background: 'var(--surface)', borderLeft: '1px solid var(--border)', padding: 40, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <div className="empty">
          <div className="empty-icon">📝</div>
          <p>未选择步骤</p>
          <p style={{ fontSize: 12 }}>点击画布上的节点查看属性</p>
        </div>
      </div>
    );
  }

  const update = (field: string, value: unknown) => {
    updateNode(node.id, { [field]: value } as Partial<DagNode>);
  };

  const handleDepsToggle = (depId: string) => {
    const deps = node.deps || [];
    const newDeps = deps.includes(depId) ? deps.filter((d) => d !== depId) : [...deps, depId];
    update('deps', newDeps);
  };

  const handleInputChange = (index: number, field: string, value: string | boolean) => {
    const inputs = [...(node.inputs || [])];
    inputs[index] = { ...inputs[index], [field]: value };
    update('inputs', inputs);
  };

  const handleOutputChange = (index: number, field: string, value: string | boolean) => {
    const outputs = [...(node.outputs || [])];
    outputs[index] = { ...outputs[index], [field]: value };
    update('outputs', outputs);
  };

  const addParam = (type: 'inputs' | 'outputs') => {
    const list = type === 'inputs' ? [...(node.inputs || [])] : [...(node.outputs || [])];
    list.push({ name: '', type: 'string', desc: '', required: false });
    update(type, list);
  };

  const removeParam = (type: 'inputs' | 'outputs', index: number) => {
    const list = type === 'inputs' ? [...(node.inputs || [])] : [...(node.outputs || [])];
    list.splice(index, 1);
    update(type, list);
  };

  return (
    <div style={{ width: 340, flexShrink: 0, background: 'var(--surface)', borderLeft: '1px solid var(--border)', display: 'flex', flexDirection: 'column', overflowY: 'auto' }}>
      <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--border)', fontWeight: 600, fontSize: 14, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <span>{t('wf.editStep')}: {node.id}</span>
        <button className="btn btn-ghost btn-xs" style={{ color: 'var(--failed)' }}
          onClick={() => { if (window.confirm(`${t('wf.deleteStepConfirm')} "${node.id}"?`)) { deleteStep(node.id); } }}>
          🗑
        </button>
      </div>

      <div style={{ padding: '12px 14px', overflowY: 'auto', flex: 1 }}>
        {/* Type */}
        <div className="form-group">
          <label>{t('step.type')}</label>
          <select className="form-input" value={node.type || 'script'}
            onChange={(e) => update('type', e.target.value)}>
            {NODE_TYPES.map((nt) => (
              <option key={nt.value} value={nt.value}>{t(nt.label)}</option>
            ))}
          </select>
        </div>

        {/* Runtime */}
        <div className="form-group">
          <label>{t('wf.runtime')}</label>
          <select className="form-input" value={node.runtime || 'python'}
            onChange={(e) => update('runtime', e.target.value)}>
            <option value="python">Python</option>
            <option value="shell">Shell</option>
            <option value="node">Node.js</option>
          </select>
        </div>

        {/* Script */}
        <div className="form-group">
          <label>{t('wf.scriptPath')}</label>
          <input className="form-input" value={node.script || ''}
            onChange={(e) => update('script', e.target.value)} />
        </div>

        {/* Target */}
        <div className="form-group">
          <label>{t('wf.target')}</label>
          <select className="form-input" value={node.target || 'local'}
            onChange={(e) => update('target', e.target.value)}>
            <option value="local">Local</option>
          </select>
        </div>

        {/* Mode */}
        <div className="form-group">
          <label>模式</label>
          <select className="form-input" value={node.mode || 'cli'}
            onChange={(e) => update('mode', e.target.value)}>
            <option value="cli">CLI</option>
            <option value="function">Function</option>
          </select>
        </div>

        {node.mode === 'function' && (
          <div className="form-group">
            <label>入口函数</label>
            <input className="form-input" value={node.entry_function || ''}
              onChange={(e) => update('entry_function', e.target.value)}
              placeholder="main" />
          </div>
        )}

        {/* Dependencies */}
        <div className="form-group">
          <label>{t('wf.deps')}</label>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
            {nodes.filter((n) => n.id !== node.id).map((n) => {
              const isDep = (node.deps || []).includes(n.id);
              return (
                <span key={n.id}
                  onClick={() => handleDepsToggle(n.id)}
                  style={{
                    padding: '2px 8px', borderRadius: 100, cursor: 'pointer', fontSize: 11,
                    background: isDep ? 'var(--primary)' : 'var(--bg)',
                    color: isDep ? '#fff' : 'var(--text-muted)',
                    border: `1px solid ${isDep ? 'var(--primary)' : 'var(--border)'}`,
                  }}>
                  {n.id}
                </span>
              );
            })}
            {nodes.filter((n) => n.id !== node.id).length === 0 && (
              <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>无其他步骤</span>
            )}
          </div>
        </div>

        {/* Condition fields */}
        {node.type === 'condition' && (
          <>
            <div className="form-group">
              <label>{t('step.condition')}</label>
              <input className="form-input" value={node.condition || ''}
                onChange={(e) => update('condition', e.target.value)}
                placeholder="$.step_a.data.count > 0" />
            </div>
            <div className="form-group">
              <label>{t('step.trueBranch')}</label>
              <input className="form-input" value={node.true_branch || ''}
                onChange={(e) => update('true_branch', e.target.value)} />
            </div>
            <div className="form-group">
              <label>{t('step.falseBranch')}</label>
              <input className="form-input" value={node.false_branch || ''}
                onChange={(e) => update('false_branch', e.target.value)} />
            </div>
          </>
        )}

        {/* Loop fields */}
        {node.type === 'loop' && (
          <div className="form-group">
            <label>{t('step.loopOver')}</label>
            <input className="form-input" value={node.loop_over || ''}
              onChange={(e) => update('loop_over', e.target.value)}
              placeholder="$.step_a.data.items" />
          </div>
        )}

        {/* Inputs */}
        <div className="form-group">
          <label style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            {t('step.inputs')}
            <button className="btn btn-ghost btn-xs" onClick={() => addParam('inputs')}>+</button>
          </label>
          {(node.inputs || []).map((p, i) => (
            <div key={i} style={{ display: 'flex', gap: 4, marginBottom: 4 }}>
              <input className="form-input" style={{ flex: 1, fontSize: 11, padding: '4px 6px' }}
                value={p.name} placeholder={t('step.paramName')}
                onChange={(e) => handleInputChange(i, 'name', e.target.value)} />
              <select className="form-input" style={{ width: 70, fontSize: 11, padding: '4px 4px' }}
                value={p.type} onChange={(e) => handleInputChange(i, 'type', e.target.value)}>
                <option value="string">string</option>
                <option value="number">number</option>
                <option value="boolean">boolean</option>
                <option value="object">object</option>
                <option value="array">array</option>
              </select>
              <button className="btn btn-ghost btn-xs" onClick={() => removeParam('inputs', i)}
                style={{ color: 'var(--failed)', padding: '2px 4px' }}>✕</button>
            </div>
          ))}
        </div>

        {/* Outputs */}
        <div className="form-group">
          <label style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            {t('step.outputs')}
            <button className="btn btn-ghost btn-xs" onClick={() => addParam('outputs')}>+</button>
          </label>
          {(node.outputs || []).map((p, i) => (
            <div key={i} style={{ display: 'flex', gap: 4, marginBottom: 4 }}>
              <input className="form-input" style={{ flex: 1, fontSize: 11, padding: '4px 6px' }}
                value={p.name} placeholder={t('step.paramName')}
                onChange={(e) => handleOutputChange(i, 'name', e.target.value)} />
              <select className="form-input" style={{ width: 70, fontSize: 11, padding: '4px 4px' }}
                value={p.type} onChange={(e) => handleOutputChange(i, 'type', e.target.value)}>
                <option value="string">string</option>
                <option value="number">number</option>
                <option value="boolean">boolean</option>
                <option value="object">object</option>
                <option value="array">array</option>
              </select>
              <button className="btn btn-ghost btn-xs" onClick={() => removeParam('outputs', i)}
                style={{ color: 'var(--failed)', padding: '2px 4px' }}>✕</button>
            </div>
          ))}
        </div>
      </div>

      {/* Save button */}
      <div style={{ padding: '10px 14px', borderTop: '1px solid var(--border)' }}>
        <button className="btn btn-primary btn-sm" style={{ width: '100%' }}
          onClick={() => saveStep(node.id)}>
          {t('wf.save')}
        </button>
      </div>
    </div>
  );
}
