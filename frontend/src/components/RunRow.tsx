import { useState } from 'react'
import type { WorkflowRun } from '../types'
import { useRunJobs } from '../hooks/useApi'
import StatusBadge from './StatusBadge'
import { relativeTime, duration } from '../utils'

interface RunRowProps {
  run: WorkflowRun
  owner: string
  repo: string
}

export default function RunRow({ run, owner, repo }: RunRowProps) {
  const [expanded, setExpanded] = useState(false)
  const { data: jobs, isLoading: jobsLoading } = useRunJobs(owner, repo, expanded ? run.id : null)

  return (
    <div className="border border-gray-700 rounded mb-2">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-3 px-4 py-3 text-left bg-gray-800 hover:bg-gray-750 transition-colors rounded cursor-pointer"
      >
        <StatusBadge status={run.status} conclusion={run.conclusion} />
        <span className="text-sm font-mono text-gray-300">#{run.run_number}</span>
        <span className="text-sm text-gray-400">{run.head_branch}</span>
        <span className="text-xs text-gray-500">{run.event}</span>
        <span className="text-xs text-gray-500">{run.actor}</span>
        <span className="ml-auto text-xs text-gray-500">
          {duration(run.created_at, run.updated_at)}
        </span>
        <span className="text-xs text-gray-600">{relativeTime(run.updated_at)}</span>
        <span className="text-gray-500 text-xs">{expanded ? '\u25B2' : '\u25BC'}</span>
      </button>

      {expanded && (
        <div className="px-4 py-3 bg-gray-850 border-t border-gray-700">
          {jobsLoading ? (
            <p className="text-sm text-gray-500">Loading jobs...</p>
          ) : jobs && jobs.length > 0 ? (
            <div className="space-y-3">
              {jobs.map((job) => (
                <div key={job.id}>
                  <div className="flex items-center gap-2 mb-1">
                    <StatusBadge status={job.status} conclusion={job.conclusion} />
                    <span className="text-sm text-white">{job.name}</span>
                  </div>
                  {job.steps && job.steps.length > 0 && (
                    <div className="ml-6 space-y-0.5">
                      {job.steps.map((step) => (
                        <div key={step.number} className="flex items-center gap-2 text-xs text-gray-400">
                          <span>
                            {step.conclusion === 'success' ? '\u2713' :
                              step.conclusion === 'failure' ? '\u2717' :
                              step.status === 'in_progress' ? '\u25CF' : '\u2022'}
                          </span>
                          <span>{step.name}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-gray-500">No jobs found</p>
          )}
        </div>
      )}
    </div>
  )
}
