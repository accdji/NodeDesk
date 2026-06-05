export interface ApiErrorBody {
  error: string;
}

export interface CreateProjectReq {
  name: string;
  id?: string;
  workflow?: string;
}

export interface CreateWorkflowReq {
  name: string;
}

export interface CloneWorkflowReq {
  name: string;
}

export interface StepAddReq {
  plugin: string;
  type?: string;
  target?: string;
  server?: string;
  runtime?: string;
  script?: string;
  mode?: string;
  entry_function?: string;
  config?: Record<string, unknown>;
  depends_on?: string[];
  inputs?: { name: string; type: string; desc: string; required: boolean }[];
  outputs?: { name: string; type: string; desc: string; required: boolean }[];
  condition?: string;
  loop_over?: string;
  true_branch?: string;
  false_branch?: string;
}

export interface BatchDeleteReq {
  ids: string[];
}

export interface PlanItem {
  index: number;
  plugin: string;
  target: string;
  depends_on: string[];
}

export interface DryRunResult {
  plan: PlanItem[];
  run_id: string;
  dry_run: boolean;
}

export interface RunResult {
  run_id: string;
  status: string;
}
