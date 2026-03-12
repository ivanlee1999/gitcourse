import { Link, useParams } from 'react-router-dom'
import { useWorkflowRuns } from '../hooks/useApi'
import RunRow from '../components/RunRow'

export default function WorkflowDetail() {
  const { owner, repo, workflowId } = useParams<{
    owner: string
    repo: string
    workflowId: string
  }>()

  const { data: runs, isLoading, error } = useWorkflowRuns(owner!, repo!, workflowId!)

  return (
    <div>
      <div className="mb-6">
        <Link to="/" className="text-sm text-gray-400 hover:text-white transition-colors">
          &larr; Dashboard
        </Link>
        <div className="flex items-center gap-2 mt-2 text-sm text-gray-400">
          <span>{owner}/{repo}</span>
          <span>&gt;</span>
          <span className="text-white">Workflow #{workflowId}</span>
        </div>
      </div>

      {isLoading && <p className="text-gray-400">Loading runs...</p>}
      {error && <p className="text-red-400">Error: {(error as Error).message}</p>}

      {runs && runs.length === 0 && (
        <p className="text-gray-500">No runs found for this workflow.</p>
      )}

      {runs && runs.map((run) => (
        <RunRow key={run.id} run={run} owner={owner!} repo={repo!} />
      ))}
    </div>
  )
}
