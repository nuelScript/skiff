import { useInfiniteQuery } from '@tanstack/react-query'
import { deploysService, type DeployCursor } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

export const APP_DEPLOYS_PAGE = 30

// One app's build history, keyset-paginated for the project Deployments tab.
export function useAppDeploys(app: string) {
  return useInfiniteQuery({
    queryKey: queryKeys.deploysPaged(app),
    queryFn: ({ pageParam }) => deploysService.list(app, pageParam, APP_DEPLOYS_PAGE),
    initialPageParam: undefined as DeployCursor | undefined,
    getNextPageParam: (last) =>
      last.length === APP_DEPLOYS_PAGE
        ? { before: last[last.length - 1].started, beforeId: last[last.length - 1].id }
        : undefined,
    enabled: !!app,
    refetchInterval: 5000,
  })
}
