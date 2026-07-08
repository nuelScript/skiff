import { useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { domainsService, type DomainsResponse } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

export function useDomains() {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery<DomainsResponse>({
    queryKey: queryKeys.domains,
    queryFn: () => domainsService.list(),
    refetchInterval: 15000, // pick up DNS/cert propagation
  })
  const domains = data?.domains ?? []
  const serverIp = data?.serverIp ?? ''

  const reload = useCallback(
    () => qc.invalidateQueries({ queryKey: queryKeys.domains }),
    [qc],
  )

  const add = useCallback(
    async (app: string, host: string, branch?: string) => {
      await domainsService.add(app, host, branch)
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

  return { domains, serverIp, isLoading, add, remove }
}
