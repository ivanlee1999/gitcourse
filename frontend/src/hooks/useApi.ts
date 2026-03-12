import { useQuery } from '@tanstack/react-query'
import type { RepoWorkflows, WorkflowRun, WorkflowJob } from '../types'

async function fetchJson<T>(url: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${res.statusText}`)
  }
  return res.json()
}

export function useDashboard() {
  return useQuery<RepoWorkflows[]>({
    queryKey: ['dashboard'],
    queryFn: () => fetchJson<RepoWorkflows[]>('/api/dashboard'),
    refetchInterval: 30000,
  })
}

export function useWorkflows(owner: string, repo: string) {
  return useQuery({
    queryKey: ['workflows', owner, repo],
    queryFn: () => fetchJson(`/api/repos/${owner}/${repo}/workflows`),
  })
}

export function useWorkflowRuns(owner: string, repo: string, workflowId: string) {
  return useQuery<WorkflowRun[]>({
    queryKey: ['workflowRuns', owner, repo, workflowId],
    queryFn: () => fetchJson<WorkflowRun[]>(`/api/repos/${owner}/${repo}/workflows/${workflowId}/runs`),
    refetchInterval: 10000,
  })
}

export function useRunJobs(owner: string, repo: string, runId: number | null) {
  return useQuery<WorkflowJob[]>({
    queryKey: ['runJobs', owner, repo, runId],
    queryFn: () => fetchJson<WorkflowJob[]>(`/api/runs/${owner}/${repo}/${runId}/jobs`),
    enabled: runId !== null,
  })
}
