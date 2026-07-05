import { useQuery } from '@tanstack/react-query'
import { analyticsService, type Analytics } from '@/services/api.service'

export function useAnalytics(rangeMins: number, app: string) {
  return useQuery<Analytics>({
    queryKey: ['analytics', rangeMins, app],
    queryFn: () => analyticsService.overview(rangeMins, app),
    refetchInterval: 10000,
  })
}
