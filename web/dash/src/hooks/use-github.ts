import { useCallback, useEffect, useState } from 'react'
import { api, type GithubStatus, type Repo } from '@/services/api.service'

export function useGithub(active: boolean) {
  const [status, setStatus] = useState<GithubStatus | null>(null)
  const [repos, setRepos] = useState<Repo[]>([])
  const [loadingRepos, setLoadingRepos] = useState(false)

  const refresh = useCallback(() => {
    api.github
      .status()
      .then(setStatus)
      .catch(() => setStatus({ configured: false, installed: false }))
  }, [])

  const loadRepos = useCallback(() => {
    setLoadingRepos(true)
    api.github
      .repos()
      .then(setRepos)
      .catch(() => setRepos([]))
      .finally(() => setLoadingRepos(false))
  }, [])

  useEffect(() => {
    if (active) refresh()
  }, [active, refresh])

  useEffect(() => {
    if (active && status?.installed) loadRepos()
  }, [active, status?.installed, loadRepos])

  return { status, repos, loadingRepos, refresh, loadRepos }
}
