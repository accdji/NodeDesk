export interface RunRecord {
  id: string;
  project_name: string;
  workflow_name: string;
  status: 'running' | 'success' | 'failed' | 'cancelled';
  created_at: string;
  finished_at?: string;
}

export interface StepRecord {
  run_id: string;
  step_name: string;
  state: 'pending' | 'running' | 'success' | 'failed' | 'skipped';
  data?: string;
  error?: string;
  duration: number;
  target: string;
}

export interface LogEntry {
  run_id: string;
  step_name: string;
  timestamp: string;
  level: string;
  message: string;
  _idx?: string;
}

export interface RunDoc {
  run: RunRecord;
  steps: StepRecord[];
  logs: LogEntry[];
}

export interface HistoryResult {
  runs: RunRecord[];
  total: number;
  page: number;
  size: number;
}

export interface HistoryFilter {
  status?: string;
  project?: string;
  q?: string;
  page: number;
  size: number;
}
