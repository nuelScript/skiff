import { useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { projectsService, type App } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'
import { useConfirm } from '@/providers/confirm-provider'

export function useApps() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const {
    data: apps = [],
    isPending,
    isError,
  } = useQuery<App[]>({
    queryKey: queryKeys.apps,
    queryFn: () => projectsService.list(),
    refetchInterval: 4000,
  })

  const reload = useCallback(() => qc.invalidateQueries({ queryKey: queryKeys.apps }), [qc])

  const stop = useCallback(
    async (name: string) => {
      if (!(await confirm({ title: `Stop ${name}?`, confirmText: 'Stop', destructive: true })))
        return
      await projectsService.stop(name)
      reload()
    },
    [reload, confirm],
  )

  return { apps, isPending, isError, reload, stop }
}
