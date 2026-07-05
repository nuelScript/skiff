import { useCallback, useEffect, useState } from 'react'
import { api, type Me } from '@/services/api.service'

export type AuthState = 'checking' | 'setup' | 'out' | 'in'

export function useAuth() {
  const [state, setState] = useState<AuthState>('checking')
  const [me, setMe] = useState<Me | null>(null)

  const refresh = useCallback(async () => {
    try {
      const m = await api.auth.me()
      setMe(m)
      setState(m.authenticated ? 'in' : m.needsSetup ? 'setup' : 'out')
    } catch {
      setState('out')
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const setup = useCallback(
    async (secret: string, email: string, name: string, password: string) => {
      await api.auth.setup(secret, email, name, password)
      await refresh()
    },
    [refresh],
  )

  const login = useCallback(
    async (email: string, password: string) => {
      await api.auth.login(email, password)
      await refresh()
    },
    [refresh],
  )

  const logout = useCallback(async () => {
    await api.auth.logout()
    setMe(null)
    setState('out')
  }, [])

  const switchTeam = useCallback(
    async (team: string) => {
      await api.auth.switchTeam(team)
      await refresh()
    },
    [refresh],
  )

  return { state, me, setup, login, logout, switchTeam, refresh }
}
