import {
  deployUrl,
  deployLogUrl,
  githubDeployUrl,
  logsUrl,
  openStream,
} from '@/services/stream.service'
import { useCallback, useRef, useState } from 'react'

export type Stream = { title: string; lines: string[] }

// Owns the live build/log drawer: one active SSE stream at a time.
export function useConsole(onDeployed: () => void) {
  const [stream, setStream] = useState<Stream | null>(null)
  const es = useRef<EventSource | null>(null)

  const start = useCallback((title: string, url: string, onDone?: () => void) => {
    es.current?.close()
    setStream({ title, lines: [] })
    es.current = openStream(url, {
      onLine: (line) =>
        setStream((s) => (s ? { ...s, lines: [...s.lines, line] } : s)),
      onDone: (ok) => {
        setStream((s) =>
          s ? { ...s, lines: [...s.lines, ok ? '✓ done' : '✗ failed'] } : s,
        )
        onDone?.()
      },
    })
  }, [])

  const close = useCallback(() => {
    es.current?.close()
    es.current = null
    setStream(null)
  }, [])

  const deploy = useCallback(
    (git: string, name: string, port: string, token?: string) =>
      start('Deploying ' + name, deployUrl(git, name, port, token), onDeployed),
    [start, onDeployed],
  )

  const deployRepo = useCallback(
    (
      repo: string,
      clone: string,
      branch: string,
      name: string,
      port: string,
      auto: boolean,
      rootDir: string,
    ) =>
      start(
        'Deploying ' + name,
        githubDeployUrl(repo, clone, branch, name, port, auto, rootDir),
        onDeployed,
      ),
    [start, onDeployed],
  )

  const showLogs = useCallback(
    (name: string) => start('Logs: ' + name, logsUrl(name)),
    [start],
  )

  const showBuildLog = useCallback(
    (app: string, id: string) => start('Build · ' + app, deployLogUrl(app, id)),
    [start],
  )

  return { stream, close, deploy, deployRepo, showLogs, showBuildLog }
}
