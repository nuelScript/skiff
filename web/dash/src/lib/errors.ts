export function errText(e: unknown, fallback = 'Something went wrong'): string {
  const data = (e as { response?: { data?: unknown } })?.response?.data
  if (typeof data === 'string' && data.trim()) return data.trim()
  return fallback
}
