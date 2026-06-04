import { api } from './client';
import type { DryRunResult, RunResult } from '../types';

export const runProject = (projectName: string, dry = false, fromStep?: string) => {
  const params = new URLSearchParams();
  if (dry) params.set('dry', '1');
  if (fromStep) params.set('from', fromStep);
  return api.post<DryRunResult | RunResult>(
    `/api/run/${encodeURIComponent(projectName)}?${params.toString()}`
  );
};

export const cancelRun = (runId: string) =>
  api.post<{ status: string }>(`/api/cancel/${encodeURIComponent(runId)}`);
