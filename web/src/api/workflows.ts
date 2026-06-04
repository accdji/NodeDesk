import { api } from './client';
import type { CreateWorkflowReq, CloneWorkflowReq } from '../types';

interface WorkflowListItem {
  name: string;
  label: string;
  steps: string[];
}

export const getWorkflows = () => api.get<WorkflowListItem[]>('/api/workflows');

export const createWorkflow = (data: CreateWorkflowReq) =>
  api.post<{ status: string }>('/api/workflows', data);

export const cloneWorkflow = (name: string, data: CloneWorkflowReq) =>
  api.post<{ status: string }>(`/api/workflows/${encodeURIComponent(name)}/clone`, data);
