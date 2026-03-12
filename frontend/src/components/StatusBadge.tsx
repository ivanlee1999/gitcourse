interface StatusBadgeProps {
  status: string;
  conclusion: string | null;
}

function getStatusColor(status: string, conclusion: string | null): string {
  if (status === 'in_progress') return 'bg-amber-500 animate-pulse'
  if (status === 'queued' || status === 'waiting' || status === 'pending') return 'bg-gray-500'

  const value = conclusion ?? status
  switch (value) {
    case 'success':
      return 'bg-green-500'
    case 'failure':
      return 'bg-red-500'
    case 'cancelled':
      return 'bg-gray-500'
    case 'skipped':
      return 'bg-gray-500'
    default:
      return 'bg-gray-500'
  }
}

function getLabel(status: string, conclusion: string | null): string {
  if (status === 'in_progress') return 'in progress'
  if (status === 'queued') return 'queued'
  if (status === 'waiting') return 'waiting'
  return conclusion ?? status
}

export default function StatusBadge({ status, conclusion }: StatusBadgeProps) {
  const color = getStatusColor(status, conclusion)
  const label = getLabel(status, conclusion)

  return (
    <span className="inline-flex items-center gap-1.5 text-xs">
      <span className={`inline-block w-2.5 h-2.5 rounded-full ${color}`} />
      <span className="text-gray-300">{label}</span>
    </span>
  )
}
