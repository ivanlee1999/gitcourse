export function relativeTime(dateStr: string): string {
  const now = Date.now()
  const then = new Date(dateStr).getTime()
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

export function duration(startStr: string, endStr: string): string {
  const start = new Date(startStr).getTime()
  const end = new Date(endStr).getTime()
  const diffMs = end - start
  const diffSec = Math.floor(diffMs / 1000)
  const min = Math.floor(diffSec / 60)
  const sec = diffSec % 60

  if (min === 0) return `${sec}s`
  return `${min}m ${sec}s`
}
