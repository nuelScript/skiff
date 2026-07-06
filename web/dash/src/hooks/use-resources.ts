import { useQuery } from '@tanstack/react-query'
import { resourcesService, type Resources } from '@/services/api.service'

export function useResources(rangeMins: number, app: string) {
  return useQuery<Resources>({
    queryKey: ['resources', rangeMins, app],
    queryFn: () => resourcesService.overview(rangeMins, app),
    refetchInterval: 10000,
  })
}
