import { useQuery } from '@tanstack/react-query'
import { serverService, type ServerInfo } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

// Polls the box's live metrics for the Server page.
export function useServer() {
  return useQuery<ServerInfo>({
    queryKey: queryKeys.server,
    queryFn: () => serverService.info(),
    refetchInterval: 4000,
  })
}
