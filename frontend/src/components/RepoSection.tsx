import type { RepoWorkflows } from '../types'
import PipelineCard from './PipelineCard'

interface RepoSectionProps {
  repoWorkflows: RepoWorkflows
}

export default function RepoSection({ repoWorkflows }: RepoSectionProps) {
  const { owner, repo, workflows } = repoWorkflows

  return (
    <section className="mb-8">
      <h2 className="text-lg font-semibold text-white mb-3">
        {owner}/{repo}
      </h2>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
        {workflows.map((wf) => (
          <PipelineCard key={wf.id} workflow={wf} owner={owner} repo={repo} />
        ))}
      </div>
    </section>
  )
}
