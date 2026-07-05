import { useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Me } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

export type AuthState = 'checking' | 'setup' | 'out' | 'in'

export function useAuth() {
  const qc = useQueryClient()
  const { data: me = null, isPending } = useQuery<Me>({
    queryKey: queryKeys.me,
    queryFn: api.auth.me,
    retry: false,
    staleTime: Infinity,
  })

  const state: AuthState = isPending
    ? 'checking'
    : me?.authenticated
      ? 'in'
      : me?.needsSetup
        ? 'setup'
        : 'out'

  const refresh = useCallback(
    () => qc.invalidateQueries({ queryKey: queryKeys.me }),
    [qc],
  )

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
    qc.setQueryData(queryKeys.me, { authenticated: false, needsSetup: false } as Me)
  }, [qc])

  const switchTeam = useCallback(
    async (team: string) => {
      await api.auth.switchTeam(team)
      await refresh()
    },
    [qc, refresh],
  )

  return { state, me, setup, login, logout, switchTeam, refresh }
}
