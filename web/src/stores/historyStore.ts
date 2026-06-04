import { create } from 'zustand';
import type { RunRecord, HistoryFilter } from '../types';
import * as api from '../api';
import { useToastStore } from './toastStore';

interface HistoryStore {
  runs: RunRecord[];
  total: number;
  page: number;
  size: number;
  loading: boolean;
  filter: HistoryFilter;
  selectedIds: string[];
  expandedId: string | null;

  load: (page?: number) => Promise<void>;
  setFilter: (f: Partial<HistoryFilter>) => void;
  toggleSelect: (id: string) => void;
  toggleAll: () => void;
  setExpanded: (id: string | null) => void;
  deleteSingle: (id: string) => Promise<void>;
  batchDelete: () => Promise<void>;
}

export const useHistoryStore = create<HistoryStore>((set, get) => ({
  runs: [],
  total: 0,
  page: 1,
  size: 20,
  loading: false,
  filter: { page: 1, size: 20 },
  selectedIds: [],
  expandedId: null,

  load: async (page?: number) => {
    const { filter } = get();
    const p = page ?? filter.page;
    set({ loading: true, page: p });
    try {
      const result = await api.getHistory({ ...filter, page: p });
      set({ runs: result.runs, total: result.total });
    } catch {
      useToastStore.getState().add('error', '加载历史失败');
    } finally {
      set({ loading: false });
    }
  },

  setFilter: (f) => {
    set((s) => ({ filter: { ...s.filter, ...f, page: 1 } }));
  },

  toggleSelect: (id) => {
    set((s) => ({
      selectedIds: s.selectedIds.includes(id)
        ? s.selectedIds.filter((i) => i !== id)
        : [...s.selectedIds, id],
    }));
  },

  toggleAll: () => {
    const { runs, selectedIds } = get();
    if (selectedIds.length === runs.length) {
      set({ selectedIds: [] });
    } else {
      set({ selectedIds: runs.map((r) => r.id) });
    }
  },

  setExpanded: (id) => {
    set((s) => ({ expandedId: s.expandedId === id ? null : id }));
  },

  deleteSingle: async (id) => {
    try {
      await api.deleteRun(id);
      useToastStore.getState().add('success', '记录已删除');
      await get().load();
    } catch {
      useToastStore.getState().add('error', '删除失败');
    }
  },

  batchDelete: async () => {
    const { selectedIds } = get();
    if (selectedIds.length === 0) return;
    try {
      await api.batchDeleteRuns(selectedIds);
      useToastStore.getState().add('success', `已删除 ${selectedIds.length} 条`);
      set({ selectedIds: [] });
      await get().load();
    } catch {
      useToastStore.getState().add('error', '批量删除失败');
    }
  },
}));
