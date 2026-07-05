import { useQuery } from '@tanstack/react-query'
import { deploysService, type Deploy } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

// The global build feed for the Deployments page — every app, newest first.
export function useAllDeploys() {
  return useQuery<Deploy[]>({
    queryKey: queryKeys.deploysAll,
    queryFn: () => deploysService.listAll(),
    refetchInterval: 5000,
  })
}
