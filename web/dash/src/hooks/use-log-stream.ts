import { useCallback, useEffect, useRef, useState } from 'react'
import { openStream, logsUrl } from '@/services/stream.service'

const MAX_LINES = 2000

// Streams a running app's runtime logs (docker logs -f, over SSE). Resets when
// the selected app changes and tears the connection down on unmount.
export function useLogStream(app: string | null) {
  const [lines, setLines] = useState<string[]>([])
  const [live, setLive] = useState(false)
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    esRef.current?.close()
    setLines([])
    if (!app) {
      setLive(false)
      return
    }
    setLive(true)
    const es = openStream(logsUrl(app), {
      onLine: (line) =>
        setLines((ls) =>
          ls.length >= MAX_LINES ? [...ls.slice(1 - MAX_LINES), line] : [...ls, line],
        ),
      onDone: () => setLive(false),
    })
    esRef.current = es
    return () => {
      es.close()
      esRef.current = null
    }
  }, [app])

  const clear = useCallback(() => setLines([]), [])

  return { lines, live, clear }
}
