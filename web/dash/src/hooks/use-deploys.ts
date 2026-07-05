import { useCallback, useState } from 'react'
import { api, type Deploy } from '@/services/api.service'

export function useDeploys() {
  const [app, setApp] = useState<string | null>(null)
  const [deploys, setDeploys] = useState<Deploy[]>([])

  const open = useCallback((name: string) => {
    setApp(name)
    api.deploys(name).then(setDeploys).catch(() => setDeploys([]))
  }, [])

  const close = useCallback(() => {
    setApp(null)
    setDeploys([])
  }, [])

  return { app, deploys, open, close }
}
