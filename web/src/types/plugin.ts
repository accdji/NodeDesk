import type { ParamDef } from './project';

export interface PluginInfo {
  name: string;
  label: string;
  description: string;
  version: string;
  runtime: string;
  script: string;
  source: 'shared' | 'project';
  mode: 'cli' | 'function';
  entry_function: string;
  inputs: ParamDef[];
  outputs: ParamDef[];
}
