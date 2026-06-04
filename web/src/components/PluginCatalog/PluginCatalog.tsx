import type { PluginInfo } from '../../types';
import { useWorkflowStore } from '../../stores/workflowStore';
import { useTranslation } from 'react-i18next';

export function PluginCatalog() {
  const { t } = useTranslation();
  const plugins = useWorkflowStore((s) => s.sharedPlugins);
  const projectPlugins = useWorkflowStore((s) => s.projectPlugins);
  const search = useWorkflowStore((s) => s.pluginSearch);
  const tab = useWorkflowStore((s) => s.pluginTab);
  const setSearch = useWorkflowStore((s) => s.setPluginSearch);
  const setTab = useWorkflowStore((s) => s.setPluginTab);
  const addStep = useWorkflowStore((s) => s.addStep);

  const allPlugins = [...plugins, ...projectPlugins];
  const filtered = allPlugins.filter((p) => {
    if (tab === 'shared') return p.source === 'shared';
    if (tab === 'project') return p.source === 'project';
    return true;
  }).filter((p) => {
    const s = search.toLowerCase();
    return !s || p.name.toLowerCase().includes(s) || p.label.toLowerCase().includes(s)
      || p.description.toLowerCase().includes(s);
  });

  const handleClick = (plugin: PluginInfo) => {
    addStep(plugin);
  };

  return (
    <div style={{ width: 260, flexShrink: 0, display: 'flex', flexDirection: 'column', background: 'var(--surface)', borderRight: '1px solid var(--border)' }}>
      <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--border)', fontWeight: 600, fontSize: 14 }}>
        {t('wf.pluginCatalog')}
      </div>

      {/* Tabs */}
      <div className="tabs">
        {['all', 'shared', 'project'].map((tb) => (
          <button key={tb} className={`tab ${tab === tb ? 'active' : ''}`} onClick={() => setTab(tb as 'all' | 'shared' | 'project')}>
            {tb === 'all' ? t('wf.allPlugins') : tb === 'shared' ? t('wf.sharedPlugins') : t('wf.projectPlugins')}
          </button>
        ))}
      </div>

      {/* Search */}
      <div className="search-box" style={{ padding: '8px 10px' }}>
        <span className="search-icon">🔍</span>
        <input className="form-input" style={{ fontSize: 12 }}
          placeholder={t('wf.searchPlugin')}
          value={search}
          onChange={(e) => setSearch(e.target.value)} />
      </div>

      {/* Plugin list */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '4px 8px' }}>
        {filtered.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 24, color: 'var(--text-muted)', fontSize: 13 }}>
            {t('wf.noPluginFound')}
          </div>
        ) : (
          filtered.map((p) => (
            <div
              key={p.name + p.source}
              onClick={() => handleClick(p)}
              style={{
                padding: '10px 12px',
                margin: '2px 0',
                borderRadius: 6,
                cursor: 'pointer',
                transition: 'all 0.15s',
                border: '1px solid transparent',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.background = 'var(--bg)';
                e.currentTarget.style.borderColor = 'var(--border)';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.background = 'transparent';
                e.currentTarget.style.borderColor = 'transparent';
              }}
            >
              <div style={{ fontWeight: 600, fontSize: 13, marginBottom: 2 }}>{p.label || p.name}</div>
              <div style={{ fontSize: 11, color: 'var(--text-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {p.description || p.name}
              </div>
              <div style={{ display: 'flex', gap: 6, marginTop: 4 }}>
                <span style={{ fontSize: 10, background: 'var(--primary-light)', color: 'var(--primary)', padding: '1px 6px', borderRadius: 3 }}>
                  {p.runtime}
                </span>
                <span style={{ fontSize: 10, background: p.source === 'shared' ? 'var(--success-light)' : 'var(--running-light)',
                  color: p.source === 'shared' ? '#059669' : '#2563EB', padding: '1px 6px', borderRadius: 3 }}>
                  {p.source === 'shared' ? t('common.shared') : t('common.project')}
                </span>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
