import { api } from './client';
import type { Project, ProjectDetail, CreateProjectReq } from '../types';

export const getProjects = () => api.get<Project[]>('/api/projects');

export const getProject = (name: string) =>
  api.get<ProjectDetail>(`/api/projects/${encodeURIComponent(name)}`);

export const createProject = (data: CreateProjectReq) =>
  api.post<{ status: string }>('/api/projects', data);

export const toggleProject = (name: string, enabled: boolean) =>
  api.patch<{ status: string }>(`/api/projects/${encodeURIComponent(name)}`, { enabled });

export const deleteProject = (name: string) =>
  api.del<{ status: string }>(`/api/projects/${encodeURIComponent(name)}`);
