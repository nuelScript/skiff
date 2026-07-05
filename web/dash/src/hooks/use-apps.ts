import { useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type App } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

export function useApps() {
  const qc = useQueryClient()
  const { data: apps = [] } = useQuery<App[]>({
    queryKey: queryKeys.apps,
    queryFn: api.apps,
    refetchInterval: 4000,
  })

  const reload = useCallback(
    () => qc.invalidateQueries({ queryKey: queryKeys.apps }),
    [qc],
  )

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
