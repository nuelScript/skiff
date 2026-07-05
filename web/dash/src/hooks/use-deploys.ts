import { useCallback, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { deploysService, type Deploy } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

// Deploy history for whichever app the user opened. The query is keyed by app
// and only runs while one is selected.
export function useDeploys() {
  const [app, setApp] = useState<string | null>(null)

  const { data: deploys = [] } = useQuery<Deploy[]>({
    queryKey: queryKeys.deploys(app),
    queryFn: () => deploysService.list(app as string),
    enabled: !!app,
  })

  const open = useCallback((name: string) => setApp(name), [])
  const close = useCallback(() => setApp(null), [])

  return { app, deploys, open, close }
}
