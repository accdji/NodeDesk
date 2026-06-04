export interface Project {
  name: string;
  project_id: string;
  workflow: string;
  enabled: boolean;
  last_run?: {
    status: string;
    created_at: string;
    finished_at?: string;
  };
}

export interface ProjectDetail {
  name: string;
  project_id: string;
  workflow: string;
  enabled: boolean;
  workflow_label: string;
  steps: StepDef[];
}

export interface StepDef {
  plugin: string;
  type: 'script' | 'condition' | 'loop' | 'start' | 'end' | 'mapper';
  target: string;
  runtime: string;
  script: string;
  server?: string;
  mode?: 'cli' | 'function';
  entry_function?: string;
  config?: Record<string, unknown>;
  depends_on?: string[];
  inputs?: ParamDef[];
  outputs?: ParamDef[];
  condition?: string;
  loop_over?: string;
  true_branch?: string;
  false_branch?: string;
}

export interface ParamDef {
  name: string;
  type: 'string' | 'number' | 'boolean' | 'object' | 'array';
  desc: string;
  required: boolean;
}
