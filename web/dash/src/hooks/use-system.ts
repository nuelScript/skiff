import { useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { systemService, type SystemInfo } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

// The control-plane's self-deploy status + history, refreshed on an interval so
// the card reflects a self-deploy as it lands.
export function useSystem() {
  const qc = useQueryClient()
  const { data: info = null } = useQuery<SystemInfo>({
    queryKey: queryKeys.system,
    queryFn: () => systemService.info(),
    refetchInterval: 20000,
  })

  const reload = useCallback(() => qc.invalidateQueries({ queryKey: queryKeys.system }), [qc])

  return { info, reload }
}
