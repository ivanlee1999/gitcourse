export function relativeTime(dateStr: string | null | undefined): string {
  if (!dateStr) return 'unknown'
  const then = new Date(dateStr).getTime()
  if (isNaN(then)) return 'unknown'
  const now = Date.now()
  const diffMs = now - then
  const diffSec = Math.floor(diffMs / 1000)
  const diffMin = Math.floor(diffSec / 60)
  const diffHour = Math.floor(diffMin / 60)
  const diffDay = Math.floor(diffHour / 24)

  if (diffSec < 60) return 'just now'
  if (diffMin < 60) return `${diffMin} min ago`
  if (diffHour < 24) return `${diffHour} hr ago`
  return `${diffDay} day${diffDay !== 1 ? 's' : ''} ago`
}

export function duration(startStr: string | null | undefined, endStr: string | null | undefined): string {
  if (!startStr || !endStr) return '—'
  const start = new Date(startStr).getTime()
  const end = new Date(endStr).getTime()
  if (isNaN(start) || isNaN(end)) return '—'
  const diffMs = end - start
  const diffSec = Math.floor(diffMs / 1000)
  const min = Math.floor(diffSec / 60)
  const sec = diffSec % 60

  if (min === 0) return `${sec}s`
  return `${min}m ${sec}s`
}
