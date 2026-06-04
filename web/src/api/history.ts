import { api } from './client';
import type { HistoryResult, RunDoc, BatchDeleteReq } from '../types';

export const getHistory = (params: {
  status?: string;
  project?: string;
  q?: string;
  page: number;
  size: number;
}) => {
  const searchParams = new URLSearchParams();
  if (params.status) searchParams.set('status', params.status);
  if (params.project) searchParams.set('project', params.project);
  if (params.q) searchParams.set('q', params.q);
  searchParams.set('page', String(params.page));
  searchParams.set('size', String(params.size));
  return api.get<HistoryResult>(`/api/history?${searchParams.toString()}`);
};

export const getRunDetail = (id: string) =>
  api.get<RunDoc>(`/api/history/${encodeURIComponent(id)}`);

export const deleteRun = (id: string) =>
  api.del<{ status: string }>(`/api/history/${encodeURIComponent(id)}`);

export const batchDeleteRuns = (ids: string[]) =>
  api.del<{ status: string }>('/api/history/batch', { ids } as BatchDeleteReq);

export const downloadLog = (id: string) => {
  window.open(`/api/history/${encodeURIComponent(id)}/logs/download`, '_blank');
};
