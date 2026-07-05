import { useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type SystemInfo } from '@/services/api.service'

// The control-plane's self-deploy status + history, refreshed on an interval so
// the card reflects a self-deploy as it lands.
export function useSystem() {
  const qc = useQueryClient()
  const { data: info = null } = useQuery<SystemInfo>({
    queryKey: ['system'],
    queryFn: api.system,
    refetchInterval: 20000,
  })

  const reload = useCallback(
    () => qc.invalidateQueries({ queryKey: ['system'] }),
    [qc],
  )

  return { info, reload }
}
