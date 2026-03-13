import { useDashboard, useSSE, useRateLimit } from '../hooks/useApi'
import RepoSection from '../components/RepoSection'
import ActiveBuilds from '../components/ActiveBuilds'

function RateLimitIndicator() {
  const { data } = useRateLimit()

  if (!data || data.remaining > 500) return null

  const pct = Math.round((data.remaining / data.limit) * 100)
  const color = data.remaining < 100 ? 'text-red-400' : 'text-yellow-400'

  return (
    <div className={`text-xs ${color} mb-2 px-2`}>
      GitHub API: {data.remaining}/{data.limit} requests remaining ({pct}%)
      {data.remaining < 100 && ' — refresh paused'}
    </div>
  )
}

export default function Dashboard() {
  useSSE()
  const { data, isLoading, error } = useDashboard()

  if (isLoading) {
    return <p className="text-gray-400">Loading...</p>
  }

  if (error) {
    return <p className="text-red-400">Error: {(error as Error).message}</p>
  }

  if (!data || data.length === 0) {
    return <p className="text-gray-500">No repositories configured.</p>
  }

  return (
    <div>
      <RateLimitIndicator />
      <ActiveBuilds repos={data} />
      {data.map((rw) => (
        <RepoSection key={`${rw.owner}/${rw.repo}`} repoWorkflows={rw} />
      ))}
    </div>
  )
}
