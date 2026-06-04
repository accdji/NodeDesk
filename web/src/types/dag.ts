import type { StepDef } from './project';

export interface DagNode extends StepDef {
  id: string;
  label: string;
  status: 'pending' | 'running' | 'success' | 'failed' | 'skipped';
  deps: string[];
  _x?: number;
  _y?: number;
}

export interface DagEdge {
  from: string;
  to: string;
}
