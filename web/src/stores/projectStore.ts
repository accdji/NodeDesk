import { create } from 'zustand';
import type { Project } from '../types';
import * as api from '../api';
import { useToastStore } from './toastStore';

interface ProjectStore {
  projects: Project[];
  loading: boolean;
  search: string;
  page: number;
  pageSize: number;
  stats: { total: number; enabled: number; disabled: number; runs24h: number };

  load: () => Promise<void>;
  create: (name: string, id: string, workflow: string) => Promise<boolean>;
  toggleEnabled: (name: string, enabled: boolean) => Promise<void>;
  deleteProject: (name: string) => Promise<boolean>;
  setSearch: (s: string) => void;
  setPage: (p: number) => void;
}

export const useProjectStore = create<ProjectStore>((set, get) => ({
  projects: [],
  loading: false,
  search: '',
  page: 1,
  pageSize: 12,
  stats: { total: 0, enabled: 0, disabled: 0, runs24h: 0 },

  load: async () => {
    set({ loading: true });
    try {
      const projects = await api.getProjects();
      const enabled = projects.filter((p) => p.enabled).length;
      const now = Date.now();
      const runs24h = projects.filter((p) => {
        if (!p.last_run?.created_at) return false;
        return now - new Date(p.last_run.created_at).getTime() < 86400000;
      }).length;
      set({
        projects,
        stats: {
          total: projects.length,
          enabled,
          disabled: projects.length - enabled,
          runs24h,
        },
      });
    } catch {
      useToastStore.getState().add('error', '加载项目列表失败');
    } finally {
      set({ loading: false });
    }
  },

  create: async (name, id, workflow) => {
    try {
      await api.createProject({ name, id: id || undefined, workflow: workflow || undefined });
      useToastStore.getState().add('success', '项目已创建');
      await get().load();
      return true;
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '创建失败';
      useToastStore.getState().add('error', msg);
      return false;
    }
  },

  toggleEnabled: async (name, enabled) => {
    try {
      await api.toggleProject(name, enabled);
      useToastStore.getState().add('success', enabled ? '项目已启用' : '项目已停用');
      await get().load();
    } catch {
      useToastStore.getState().add('error', '操作失败');
    }
  },

  deleteProject: async (name) => {
    try {
      await api.deleteProject(name);
      useToastStore.getState().add('success', '项目已删除');
      await get().load();
      return true;
    } catch {
      useToastStore.getState().add('error', '删除失败');
      return false;
    }
  },

  setSearch: (search) => set({ search, page: 1 }),
  setPage: (page) => set({ page }),
}));
