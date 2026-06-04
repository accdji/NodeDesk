import { api } from './client';
import type { PluginInfo } from '../types';

export const getPlugins = (project?: string) =>
  api.get<PluginInfo[]>(`/api/plugins${project ? `?project=${encodeURIComponent(project)}` : ''}`);
