import { api } from './client';

export interface PipelineConfig {
  servers?: Record<string, unknown>;
  workflows?: Record<string, unknown>;
  projects?: unknown[];
  global?: { work_dir?: string };
  lang?: string;
}

export const getConfig = () =>
  api.get<{ path: string; config: PipelineConfig }>('/api/config');

export const saveConfig = (config: PipelineConfig) =>
  api.put<{ status: string }>('/api/config', config);
