export interface WorkflowRun {
  id: number;
  status: string;
  conclusion: string | null;
  created_at: string;
  updated_at: string;
  head_branch: string;
  event: string;
  actor: string;
  run_number: number;
  html_url: string;
}

export interface Workflow {
  id: number;
  name: string;
  state: string;
  latest_run: WorkflowRun | null;
}

export interface RepoWorkflows {
  repo: string;
  owner: string;
  workflows: Workflow[];
}

export interface WorkflowJob {
  id: number;
  name: string;
  status: string;
  conclusion: string | null;
  started_at: string;
  completed_at: string;
  steps: JobStep[];
}

export interface JobStep {
  name: string;
  status: string;
  conclusion: string | null;
  number: number;
  started_at: string;
  completed_at: string;
}
