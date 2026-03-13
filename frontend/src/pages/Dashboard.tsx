import { useDashboard, useSSE } from '../hooks/useApi'
import RepoSection from '../components/RepoSection'
import ActiveBuilds from '../components/ActiveBuilds'

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
      <ActiveBuilds repos={data} />
      {data.map((rw) => (
        <RepoSection key={`${rw.owner}/${rw.repo}`} repoWorkflows={rw} />
      ))}
    </div>
  )
}
