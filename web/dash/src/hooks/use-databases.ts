import { useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { databasesService, type Database, type DbEngine } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

export function useDatabases() {
  const qc = useQueryClient()
  const { data: databases = [], isPending, isError } = useQuery<Database[]>({
    queryKey: queryKeys.databases,
    queryFn: () => databasesService.list(),
    refetchInterval: 8000,
  })

  const reload = useCallback(
    () => qc.invalidateQueries({ queryKey: queryKeys.databases }),
    [qc],
  )

  const create = useCallback(
    async (engine: DbEngine, name: string) => {
      await databasesService.create(engine, name)
      reload()
    },
    [reload],
  )

  const remove = useCallback(
    async (id: string) => {
      await databasesService.remove(id)
      reload()
    },
    [reload],
  )

  const attach = useCallback(
    async (id: string, app: string) => {
      await databasesService.attach(id, app)
      reload()
    },
    [reload],
  )

  const detach = useCallback(
    async (id: string, app: string) => {
      await databasesService.detach(id, app)
      reload()
    },
    [reload],
  )

  const setPublic = useCallback(
    async (id: string, on: boolean) => {
      await databasesService.setPublic(id, on)
      reload()
    },
    [reload],
  )

  return { databases, isPending, isError, create, remove, attach, detach, setPublic }
}
