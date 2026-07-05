import { api, type App } from '@/services/api.service'
import { useCallback, useEffect, useState } from 'react'

export function useApps() {
  const [apps, setApps] = useState<App[]>([])

  const reload = useCallback(() => {
    api.apps().then(setApps).catch(() => { })
  }, [])

  useEffect(() => {
    reload()
    const t = setInterval(reload, 4000)
    return () => clearInterval(t)
  }, [reload])

  const stop = useCallback(
    async (name: string) => {
      if (!confirm('Stop ' + name + '?')) return
      await api.down(name)
      reload()
    },
    [reload],
  )

  return { apps, reload, stop }
}
