import { useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { storageService, type Bucket } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

export function useStorage() {
  const qc = useQueryClient()
  const {
    data: buckets = [],
    isPending,
    isError,
  } = useQuery<Bucket[]>({
    queryKey: queryKeys.storage,
    queryFn: () => storageService.list(),
    refetchInterval: 8000,
  })

  const reload = useCallback(() => qc.invalidateQueries({ queryKey: queryKeys.storage }), [qc])

  const create = useCallback(
    async (name: string) => {
      await storageService.create(name)
      reload()
    },
    [reload],
  )

  const remove = useCallback(
    async (id: string) => {
      await storageService.remove(id)
      reload()
    },
    [reload],
  )

  const attach = useCallback(
    async (id: string, app: string) => {
      await storageService.attach(id, app)
      reload()
    },
    [reload],
  )

  const detach = useCallback(
    async (id: string, app: string) => {
      await storageService.detach(id, app)
      reload()
    },
    [reload],
  )

  return { buckets, isPending, isError, create, remove, attach, detach }
}
