export type StreamHandlers = {
  onLine: (line: string) => void
  onDone: (ok: boolean) => void
}

// Wraps an SSE endpoint; closes on the [done] sentinel or on error.
export function openStream(url: string, h: StreamHandlers): EventSource {
  const es = new EventSource(url)
  es.onmessage = (ev) => {
    if (ev.data.startsWith('[done]')) {
      es.close()
      h.onDone(ev.data.includes('ok'))
      return
    }
    h.onLine(ev.data)
  }
  es.onerror = () => es.close()
  return es
}

const q = (s: string) => encodeURIComponent(s)

export const deployUrl = (
  git: string,
  name: string,
  port: string,
  token = '',
) =>
  `/api/deploy?git=${q(git)}&name=${q(name)}&port=${q(port)}` +
  (token ? `&token=${q(token)}` : '')

export const logsUrl = (name: string) => `/api/logs?app=${q(name)}`

export const githubDeployUrl = (
  repo: string,
  clone: string,
  branch: string,
  name: string,
  port: string,
  auto: boolean,
  rootDir = '',
) =>
  `/api/github/deploy?repo=${q(repo)}&clone=${q(clone)}&branch=${q(branch)}` +
  `&name=${q(name)}&port=${q(port)}&auto=${auto ? '1' : '0'}&rootdir=${q(rootDir)}`

export const deployLogUrl = (app: string, id: string) =>
  `/api/deploys/log?app=${q(app)}&id=${q(id)}`

export const redeployUrl = (name: string) => `/api/redeploy?app=${q(name)}`

export const rollbackUrl = (app: string, id: string) =>
  `/api/rollback?app=${q(app)}&id=${q(id)}`

export const previewUrl = (app: string, branch: string) =>
  `/api/preview?app=${q(app)}&branch=${q(branch)}`
