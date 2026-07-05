import { useQuery } from '@tanstack/react-query'
import { analyticsService, type Analytics } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

export function useAnalytics() {
  return useQuery<Analytics>({
    queryKey: queryKeys.analytics,
    queryFn: () => analyticsService.overview(),
    refetchInterval: 10000,
  })
}
