import { useInfiniteQuery } from '@tanstack/react-query'
import { deploysService, type DeployCursor } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

export const DEPLOYS_PAGE_SIZE = 30

// The global build feed for the Deployments page — every app, newest first,
// keyset-paginated. Each "Load more" fetches the next older page; the list
// stays live on an interval. A full page means there may be more to load.
export function useAllDeploys() {
  return useInfiniteQuery({
    queryKey: queryKeys.deploysAll,
    queryFn: ({ pageParam }) => deploysService.listAll(pageParam, DEPLOYS_PAGE_SIZE),
    initialPageParam: undefined as DeployCursor | undefined,
    getNextPageParam: (last) =>
      last.length === DEPLOYS_PAGE_SIZE
        ? { before: last[last.length - 1].started, beforeId: last[last.length - 1].id }
        : undefined,
    refetchInterval: 5000,
  })
}
