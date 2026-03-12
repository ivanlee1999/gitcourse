import { useDashboard } from '../hooks/useApi'
import RepoSection from '../components/RepoSection'

export default function Dashboard() {
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

  const sorted = [...data].sort((a, b) =>
    `${a.owner}/${a.repo}`.localeCompare(`${b.owner}/${b.repo}`)
  )

  return (
    <div>
      {sorted.map((rw) => (
        <RepoSection key={`${rw.owner}/${rw.repo}`} repoWorkflows={rw} />
      ))}
    </div>
  )
}
