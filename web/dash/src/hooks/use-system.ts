import { useCallback, useEffect, useState } from 'react'
import { api, type SystemInfo } from '@/services/api.service'

// useSystem loads the control-plane's self-deploy status + history and refreshes
// it on an interval, so the card reflects a self-deploy as it lands.
export function useSystem() {
  const [info, setInfo] = useState<SystemInfo | null>(null)

  const reload = useCallback(() => {
    api
      .system()
      .then(setInfo)
      .catch(() => setInfo(null))
  }, [])

  useEffect(() => {
    reload()
    const id = window.setInterval(reload, 20000)
    return () => window.clearInterval(id)
  }, [reload])

  return { info, reload }
}
