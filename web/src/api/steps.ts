import { api } from './client';
import type { StepAddReq } from '../types';

export const addStep = (projectName: string, step: StepAddReq) =>
  api.post<{ status: string }>(`/api/projects/${encodeURIComponent(projectName)}/steps`, step);

export const updateStep = (projectName: string, stepName: string, step: Partial<StepAddReq>) =>
  api.put<{ status: string }>(`/api/projects/${encodeURIComponent(projectName)}/steps/${encodeURIComponent(stepName)}`, step);

export const deleteStep = (projectName: string, stepName: string) =>
  api.del<{ status: string }>(`/api/projects/${encodeURIComponent(projectName)}/steps/${encodeURIComponent(stepName)}`);
