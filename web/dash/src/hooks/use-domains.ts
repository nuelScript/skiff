import { useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { domainsService, type Domain } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

export function useDomains() {
  const qc = useQueryClient()
  const { data: domains = [], isLoading } = useQuery<Domain[]>({
    queryKey: queryKeys.domains,
    queryFn: () => domainsService.list(),
    refetchInterval: 15000, // pick up DNS/cert propagation
  })

  const reload = useCallback(
    () => qc.invalidateQueries({ queryKey: queryKeys.domains }),
    [qc],
  )

  const add = useCallback(
    async (app: string, host: string) => {
      await domainsService.add(app, host)
      reload()
    },
    [reload],
  )

  const remove = useCallback(
    async (host: string) => {
      await domainsService.remove(host)
      reload()
    },
    [reload],
  )

  return { domains, isLoading, add, remove }
}
