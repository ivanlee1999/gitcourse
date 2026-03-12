import type { RepoWorkflows } from '../types'

interface ActiveBuildsProps {
  repos: RepoWorkflows[]
}

interface ActiveBuild {
  repo: string
  workflow: string
  runner_name?: string
  current_step?: string
}

export default function ActiveBuilds({ repos }: ActiveBuildsProps) {
  const builds: ActiveBuild[] = []

  for (const rw of repos) {
    for (const wf of rw.workflows) {
      if (wf.latest_run?.status === 'in_progress') {
        builds.push({
          repo: `${rw.owner}/${rw.repo}`,
          workflow: wf.name,
          runner_name: wf.latest_run.runner_name,
          current_step: wf.latest_run.current_step,
        })
      }
    }
  }

  if (builds.length === 0) return null

  return (
    <div className="mb-6 bg-amber-500/10 border border-amber-500/30 rounded-lg px-4 py-3">
      <div className="text-xs font-semibold text-amber-400 mb-2">
        {'\uD83D\uDD04'} Active Builds ({builds.length})
      </div>
      <div className="space-y-1">
        {builds.map((b, i) => (
          <div key={i} className="text-xs text-gray-300 flex items-center gap-2 truncate">
            <span className="text-white font-medium">{b.repo}</span>
            <span className="text-gray-500">{'\u203A'}</span>
            <span>{b.workflow}</span>
            {b.runner_name && (
              <>
                <span className="text-gray-600">on</span>
                <span className="text-amber-400/80">{b.runner_name}</span>
              </>
            )}
            {b.current_step && (
              <>
                <span className="text-gray-600">{'\u2022'}</span>
                <span className="text-amber-300/70 truncate">{b.current_step}</span>
              </>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
