import { create } from 'zustand';
import type { ProjectDetail, StepDef, PluginInfo, DagNode, DagEdge, PlanItem, StepRecord, LogEntry } from '../types';
import * as api from '../api';
import { useToastStore } from './toastStore';

interface WorkflowStore {
  projectName: string;
  workflowName: string;
  workflowLabel: string;
  projectId: string;
  enabled: boolean;
  dagNodes: DagNode[];
  dagEdges: DagEdge[];
  sharedPlugins: PluginInfo[];
  projectPlugins: PluginInfo[];
  pluginSearch: string;
  pluginTab: 'all' | 'shared' | 'project';
  selectedNodeId: string | null;
  showConfigEditor: boolean;
  loading: boolean;
  running: boolean;
  resultSteps: StepRecord[];
  resultLogs: LogEntry[];
  resultRunId: string;
  resultStatus: string;
  resultPlan: PlanItem[];

  load: (projectName: string) => Promise<void>;
  loadPlugins: () => Promise<void>;
  setPluginSearch: (s: string) => void;
  setPluginTab: (t: 'all' | 'shared' | 'project') => void;
  selectNode: (id: string | null) => void;
  setConfigEditor: (v: boolean) => void;
  addStep: (plugin: PluginInfo) => Promise<void>;
  updateNode: (nodeId: string, updates: Partial<DagNode>) => void;
  addEdge: (from: string, to: string) => void;
  removeEdge: (from: string, to: string) => void;
  saveStep: (nodeId: string) => Promise<void>;
  deleteStep: (nodeId: string) => Promise<void>;
  saveConfig: (config: unknown) => Promise<void>;
  run: (dry?: boolean) => Promise<void>;
  cancelRun: () => Promise<void>;
  resetLayout: () => void;
  getNodeById: (id: string) => DagNode | undefined;
  computeLevels: () => string[][];
}

export const useWorkflowStore = create<WorkflowStore>((set, get) => ({
  projectName: '',
  workflowName: '',
  workflowLabel: '',
  projectId: '',
  enabled: true,
  dagNodes: [],
  dagEdges: [],
  sharedPlugins: [],
  projectPlugins: [],
  pluginSearch: '',
  pluginTab: 'all',
  selectedNodeId: null,
  showConfigEditor: false,
  loading: false,
  running: false,
  resultSteps: [],
  resultLogs: [],
  resultRunId: '',
  resultStatus: '',
  resultPlan: [],

  load: async (projectName) => {
    set({ loading: true, projectName });
    try {
      const detail = await api.getProject(projectName);
      const nodes: DagNode[] = detail.steps.map((s: StepDef, i: number) => {
        const sameCount = detail.steps.filter((x: StepDef, j: number) => x.plugin === s.plugin && j < i).length;
        return {
          ...s,
          id: sameCount > 0 ? `${s.plugin}#${sameCount}` : s.plugin,
          label: s.plugin,
          status: 'pending' as const,
          deps: s.depends_on || [],
        };
      });

      const edges: DagEdge[] = [];
      nodes.forEach((n) => {
        n.deps.forEach((dep: string) => {
          const depNode = nodes.find((x) => x.plugin === dep || x.id === dep);
          if (depNode) edges.push({ from: depNode.id, to: n.id });
        });
      });

      set({
        workflowName: detail.workflow,
        workflowLabel: detail.workflow_label || detail.workflow,
        projectId: detail.project_id,
        enabled: detail.enabled,
        dagNodes: nodes,
        dagEdges: edges,
        selectedNodeId: null,
        showConfigEditor: false,
        resultSteps: [],
        resultLogs: [],
        resultRunId: '',
        resultStatus: '',
        resultPlan: [],
      });
      get().loadPlugins();
    } catch {
      useToastStore.getState().add('error', '加载工作流失败');
    } finally {
      set({ loading: false });
    }
  },

  loadPlugins: async () => {
    try {
      const all = await api.getPlugins(get().projectName);
      set({
        sharedPlugins: all.filter((p) => p.source === 'shared'),
        projectPlugins: all.filter((p) => p.source === 'project'),
      });
    } catch { /* ignore */ }
  },

  setPluginSearch: (pluginSearch) => set({ pluginSearch }),
  setPluginTab: (pluginTab) => set({ pluginTab }),
  selectNode: (id) => set({ selectedNodeId: id, showConfigEditor: false }),
  setConfigEditor: (v) => set({ showConfigEditor: v, selectedNodeId: v ? null : get().selectedNodeId }),

  addStep: async (plugin) => {
    const stepAdd = {
      plugin: plugin.name,
      type: 'script',
      target: 'local',
      runtime: plugin.runtime || 'python',
      script: plugin.script || `${plugin.name}.py`,
      mode: plugin.mode || 'cli',
      entry_function: plugin.entry_function || '',
      inputs: plugin.inputs || [],
      outputs: plugin.outputs || [],
    };
    try {
      await api.addStep(get().projectName, stepAdd);
      useToastStore.getState().add('success', '步骤已添加');
      await get().load(get().projectName);
    } catch {
      useToastStore.getState().add('error', '添加步骤失败');
    }
  },

  updateNode: (nodeId, updates) => {
    set((s) => ({
      dagNodes: s.dagNodes.map((n) => (n.id === nodeId ? { ...n, ...updates } : n)),
    }));
  },

  addEdge: (from, to) => {
    set((s) => {
      const already = s.dagEdges.some((e) => e.from === from && e.to === to);
      if (already) return s;
      const toNode = s.dagNodes.find((n) => n.id === to);
      if (!toNode) return s;
      const newDeps = [...(toNode.deps || []), from];
      return {
        dagNodes: s.dagNodes.map((n) => (n.id === to ? { ...n, deps: newDeps } : n)),
        dagEdges: [...s.dagEdges, { from, to }],
      };
    });
  },

  removeEdge: (from, to) => {
    set((s) => ({
      dagNodes: s.dagNodes.map((n) =>
        n.id === to ? { ...n, deps: (n.deps || []).filter((d) => d !== from) } : n
      ),
      dagEdges: s.dagEdges.filter((e) => !(e.from === from && e.to === to)),
    }));
  },

  saveStep: async (nodeId) => {
    const node = get().dagNodes.find((n) => n.id === nodeId);
    if (!node) return;
    const stepName = node.plugin;
    try {
      await api.updateStep(get().projectName, stepName, {
        plugin: node.plugin,
        type: node.type,
        target: node.target,
        server: node.server,
        runtime: node.runtime,
        script: node.script,
        mode: node.mode,
        entry_function: node.entry_function,
        depends_on: node.deps,
        config: node.config,
        inputs: node.inputs,
        outputs: node.outputs,
        condition: node.condition,
        loop_over: node.loop_over,
        true_branch: node.true_branch,
        false_branch: node.false_branch,
      });
      useToastStore.getState().add('success', '步骤已更新');
      await get().load(get().projectName);
    } catch {
      useToastStore.getState().add('error', '保存步骤失败');
    }
  },

  deleteStep: async (nodeId) => {
    const node = get().dagNodes.find((n) => n.id === nodeId);
    if (!node) return;
    try {
      await api.deleteStep(get().projectName, node.plugin);
      useToastStore.getState().add('success', '步骤已删除');
      set({ selectedNodeId: null });
      await get().load(get().projectName);
    } catch {
      useToastStore.getState().add('error', '删除步骤失败');
    }
  },

  saveConfig: async (config) => {
    try {
      await api.saveConfig(config as api.PipelineConfig);
      useToastStore.getState().add('success', '配置已保存');
      await get().load(get().projectName);
    } catch {
      useToastStore.getState().add('error', '配置保存失败');
    }
  },

  run: async (dry) => {
    set({ running: true, resultSteps: [], resultLogs: [], resultStatus: '', resultPlan: [] });
    try {
      const result = await api.runProject(get().projectName, dry);
      if ('dry_run' in result && result.dry_run) {
        set({ resultPlan: (result as { plan: PlanItem[] }).plan, running: false });
      } else {
        const r = result as { run_id: string; status: string };
        // Execute in-place on canvas — SSE connection handled by WorkflowPage
        set({ resultRunId: r.run_id, resultStatus: 'running', running: true });
      }
    } catch {
      useToastStore.getState().add('error', '执行失败');
      set({ running: false });
    }
  },

  cancelRun: async () => {
    try {
      await api.cancelRun(get().resultRunId);
      set({ running: false, resultStatus: 'cancelled' });
    } catch { /* ignore */ }
  },

  resetLayout: () => {
    set((s) => ({
      dagNodes: s.dagNodes.map((n) => ({ ...n, _x: undefined, _y: undefined })),
    }));
  },

  getNodeById: (id) => get().dagNodes.find((n) => n.id === id),

  computeLevels: () => {
    const { dagNodes, dagEdges } = get();
    if (dagNodes.length === 0) return [];
    const inDegree: Record<string, number> = {};
    const adj: Record<string, string[]> = {};
    dagNodes.forEach((n) => {
      inDegree[n.id] = 0;
      adj[n.id] = [];
    });
    dagEdges.forEach((e) => {
      if (adj[e.from]) adj[e.from].push(e.to);
      if (inDegree[e.to] !== undefined) inDegree[e.to]++;
    });
    const levels: string[][] = [];
    let queue = Object.keys(inDegree).filter((id) => inDegree[id] === 0);
    while (queue.length > 0) {
      levels.push([...queue]);
      const next: string[] = [];
      queue.forEach((id) => {
        (adj[id] || []).forEach((child) => {
          inDegree[child]--;
          if (inDegree[child] === 0) next.push(child);
        });
      });
      queue = next;
    }
    return levels;
  },
}));
