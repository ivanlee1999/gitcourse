import { Link } from 'react-router-dom'
import type { Workflow } from '../types'
import StatusBadge from './StatusBadge'
import { relativeTime } from '../utils'

interface PipelineCardProps {
  workflow: Workflow
  owner: string
  repo: string
}

function getBorderColor(status: string, conclusion: string | null): string {
  if (status === 'in_progress') return '#f59e0b'
  const value = conclusion ?? status
  switch (value) {
    case 'success': return '#22c55e'
    case 'failure': return '#ef4444'
    default: return '#6b7280'
  }
}

export default function PipelineCard({ workflow, owner, repo }: PipelineCardProps) {
  const run = workflow.latest_run
  const borderColor = run
    ? getBorderColor(run.status, run.conclusion)
    : '#4b5563'

  return (
    <Link
      to={`/repos/${owner}/${repo}/workflows/${workflow.id}`}
      className="block bg-gray-800 border border-gray-700 rounded p-4 hover:bg-gray-700/50 hover:border-gray-600 transition-colors no-underline"
      style={{ borderLeftWidth: '4px', borderLeftColor: borderColor }}
    >
      <h3 className="text-sm font-semibold text-white truncate mb-2">{workflow.name}</h3>
      {run ? (
        <div className="space-y-1">
          <StatusBadge status={run.status} conclusion={run.conclusion} />
          <div className="flex items-center gap-2 text-xs text-gray-400">
            <span>{run.head_branch}</span>
            <span>&middot;</span>
            <span>{run.event}</span>
          </div>
          {run.status === 'in_progress' && (run.runner_name || run.current_step) && (
            <div className="space-y-0.5">
              {run.runner_name && (
                <div className="text-xs text-amber-400/80 truncate">{'\uD83D\uDDA5'} {run.runner_name}</div>
              )}
              {run.current_step && (
                <div className="text-xs text-amber-300/70 truncate">{'\u2699'} {run.current_step}</div>
              )}
            </div>
          )}
          <div className="text-xs text-gray-500">{relativeTime(run.created_at)}</div>
        </div>
      ) : (
        <p className="text-xs text-gray-500">No runs</p>
      )}
    </Link>
  )
}
