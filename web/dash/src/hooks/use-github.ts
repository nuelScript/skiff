import { useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type GithubStatus, type Repo } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

export function useGithub(active: boolean) {
  const qc = useQueryClient()

  const { data: status = null } = useQuery<GithubStatus>({
    queryKey: queryKeys.github.status,
    queryFn: api.github.status,
    enabled: active,
  })

  const { data: repos = [], isFetching: loadingRepos } = useQuery<Repo[]>({
    queryKey: queryKeys.github.repos,
    queryFn: api.github.repos,
    enabled: active && !!status?.installed,
  })

  const refresh = useCallback(
    () => qc.invalidateQueries({ queryKey: queryKeys.github.status }),
    [qc],
  )
  const loadRepos = useCallback(
    () => qc.invalidateQueries({ queryKey: queryKeys.github.repos }),
    [qc],
  )

  return { status, repos, loadingRepos, refresh, loadRepos }
}
