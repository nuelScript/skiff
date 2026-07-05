import { useState, type ReactNode } from 'react'
import { useParams, useNavigate, Link } from 'react-router'
import { ChevronRight, ExternalLink, RotateCw, RotateCcw, GitBranch } from 'lucide-react'
import { useProject } from '@/hooks/use-project'
import { useConsole } from '@/hooks/use-console'
import { projectsService } from '@/services/api.service'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EnvEditor } from '@/components/env-editor'
import { Drawer } from '@/components/drawer'

function rel(unix: number): string {
  const s = Math.max(0, Math.floor(Date.now() / 1000 - unix))
  if (s < 60) return s + 's ago'
  if (s < 3600) return Math.floor(s / 60) + 'm ago'
  if (s < 86400) return Math.floor(s / 3600) + 'h ago'
  return Math.floor(s / 86400) + 'd ago'
}

const runningPill = (state: string): string =>
  state === 'running'
    ? 'border-emerald-400/25 bg-emerald-400/10 text-emerald-300'
    : state === 'exited' || state === 'missing'
      ? 'border-rose-400/25 bg-rose-400/10 text-rose-300'
      : 'border-white/15 bg-white/5 text-muted-foreground'

const deployDot = (status: string): string =>
  status === 'live'
    ? 'bg-emerald-400'
    : status === 'failed'
      ? 'bg-rose-400'
      : 'bg-amber-400 pulse-dot'

export default function ProjectDetailPage() {
  const { name = '' } = useParams()
  const navigate = useNavigate()
  const { project, reload } = useProject(name)
  const term = useConsole(reload)

  if (!project) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center">…</div>
    )
  }

  const latest = project.deploys[0]
  const host = project.url.replace(/^https?:\/\//, '')

  return (
    <div className="px-8 py-8">
      {/* breadcrumb */}
      <div className="text-muted-foreground mb-4 flex items-center gap-1.5 text-sm">
        <Link to="/" className="hover:text-foreground transition-colors">
          Projects
        </Link>
        <ChevronRight className="h-3.5 w-3.5" />
        <span className="text-foreground font-medium">{project.name}</span>
      </div>

      {/* header */}
      <header className="mb-6 flex flex-wrap items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold tracking-tight">{project.name}</h1>
          <span
            className={
              'inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 font-mono text-[10px] font-medium tracking-wider uppercase ' +
              runningPill(project.state)
            }
          >
            <span
              className={
                'h-1.5 w-1.5 rounded-full ' +
                (project.state === 'running' ? 'bg-emerald-400' : 'bg-rose-400')
              }
            />
            {project.state}
          </span>
        </div>
        <div className="flex items-center gap-2">
          {project.repo && (
            <Button
              size="sm"
              variant="outline"
              asChild
            >
              <a href={'https://github.com/' + project.repo} target="_blank" rel="noreferrer">
                <GitBranch />
                Repository
              </a>
            </Button>
          )}
          <Button size="sm" variant="outline" onClick={() => term.redeploy(project.name)}>
            <RotateCw />
            Redeploy
          </Button>
          <Button size="sm" asChild>
            <a href={project.url} target="_blank" rel="noreferrer">
              <ExternalLink />
              Visit
            </a>
          </Button>
        </div>
      </header>

      <Tabs defaultValue="overview">
        <TabsList variant="line" className="mb-6 w-full justify-start gap-6 border-b border-white/8">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="deployments">Deployments</TabsTrigger>
          <TabsTrigger value="environment">Environment</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        {/* Overview */}
        <TabsContent value="overview" className="pb-[46vh]">
          <div className="grid gap-4 lg:grid-cols-[1.6fr_1fr]">
            <section className="rounded-xl border border-white/8 bg-linear-to-b from-white/2.5 to-transparent p-5">
              <div className="mb-4 flex items-center justify-between">
                <h2 className="text-muted-foreground font-mono text-[11px] tracking-wider uppercase">
                  Production Deployment
                </h2>
                {latest && (
                  <span
                    className={
                      'font-mono text-[10px] tracking-wider uppercase ' +
                      (latest.status === 'live'
                        ? 'text-emerald-300'
                        : latest.status === 'failed'
                          ? 'text-rose-300'
                          : 'text-amber-300')
                    }
                  >
                    {latest.status === 'live' ? 'Ready' : latest.status}
                  </span>
                )}
              </div>
              {latest ? (
                <div className="flex flex-col gap-4">
                  <a
                    href={project.url}
                    target="_blank"
                    rel="noreferrer"
                    className="group hover:text-foreground flex w-fit items-center gap-2 font-mono text-[15px] font-medium transition-colors"
                  >
                    <span className={'h-2 w-2 shrink-0 rounded-full ' + deployDot(latest.status)} />
                    {host}
                    <ExternalLink className="h-3.5 w-3.5 opacity-0 transition-opacity group-hover:opacity-100" />
                  </a>
                  <div className="flex flex-col gap-1.5 border-t border-white/5 pt-4">
                    <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                      {project.repo ? (
                        <a
                          href={'https://github.com/' + project.repo + '/commit/' + latest.commit}
                          target="_blank"
                          rel="noreferrer"
                          className="font-mono text-sm hover:underline"
                        >
                          {latest.commit || '—'}
                        </a>
                      ) : (
                        <span className="font-mono text-sm">{latest.commit || '—'}</span>
                      )}
                      <span className="text-muted-foreground font-mono text-xs">
                        on {project.branch || 'main'} · {latest.trigger} · {rel(latest.started)}
                      </span>
                    </div>
                    {latest.message && <p className="text-foreground/90 text-sm">{latest.message}</p>}
                  </div>
                </div>
              ) : (
                <p className="text-muted-foreground text-sm">
                  No deployments yet — push to{' '}
                  <span className="font-mono">{project.branch || 'main'}</span> or hit Redeploy.
                </p>
              )}
            </section>

            <section className="rounded-xl border border-white/8 bg-linear-to-b from-white/2.5 to-transparent p-5">
              <h2 className="text-muted-foreground mb-2 font-mono text-[11px] tracking-wider uppercase">
                Source
              </h2>
              <dl className="divide-y divide-white/5">
                <Row label="Repository">
                  {project.repo ? (
                    <a
                      href={'https://github.com/' + project.repo}
                      target="_blank"
                      rel="noreferrer"
                      className="font-mono text-xs hover:underline"
                    >
                      {project.repo}
                    </a>
                  ) : (
                    <span className="font-mono text-xs">—</span>
                  )}
                </Row>
                <Row label="Branch">
                  <span className="font-mono text-xs">{project.branch || 'main'}</span>
                </Row>
                <Row label="Root directory">
                  <span className="font-mono text-xs">{project.rootDir || '/'}</span>
                </Row>
                <Row label="Port">
                  <span className="font-mono text-xs">{project.port}</span>
                </Row>
                <Row label="Auto-deploy">
                  <span
                    className={
                      'font-mono text-xs ' +
                      (project.auto ? 'text-emerald-300' : 'text-muted-foreground')
                    }
                  >
                    {project.auto ? 'On' : 'Off'}
                  </span>
                </Row>
              </dl>
            </section>
          </div>
        </TabsContent>

        {/* Deployments */}
        <TabsContent value="deployments" className="pb-[46vh]">
          <div className="overflow-hidden rounded-xl border border-white/8">
            {project.deploys.length === 0 ? (
              <p className="text-muted-foreground p-8 text-center text-sm">No deployments yet.</p>
            ) : (
              project.deploys.map((d) => (
                <div
                  key={d.id}
                  className="group flex items-center gap-2 border-b border-white/5 pr-3 transition-colors last:border-0 hover:bg-white/[0.03]"
                >
                  <button
                    onClick={() => term.showBuildLog(project.name, d.id)}
                    className="flex min-w-0 flex-1 items-center gap-4 px-5 py-3.5 text-left"
                  >
                    <span className={'h-2 w-2 shrink-0 rounded-full ' + deployDot(d.status)} />
                    <span className="min-w-0 flex-1 truncate text-sm">
                      {d.message || d.commit || d.status}
                    </span>
                    {d.commit && (
                      <span className="text-muted-foreground shrink-0 font-mono text-xs">
                        {d.commit}
                      </span>
                    )}
                    <span className="text-muted-foreground w-16 shrink-0 font-mono text-[11px]">
                      {d.trigger}
                    </span>
                    <span className="text-muted-foreground w-16 shrink-0 text-right font-mono text-[11px]">
                      {rel(d.started)}
                    </span>
                  </button>
                  {d.rollbackable && (
                    <button
                      onClick={() => {
                        if (
                          confirm(
                            `Roll back ${project.name} to this build? It re-runs instantly — no rebuild.`,
                          )
                        )
                          term.rollback(project.name, d.id)
                      }}
                      title="Instant rollback to this build"
                      className="text-muted-foreground hover:text-foreground flex shrink-0 items-center gap-1 rounded-[6px] border border-white/10 px-2 py-1 text-[11px] opacity-0 transition group-hover:opacity-100 hover:border-white/20"
                    >
                      <RotateCcw className="h-3 w-3" />
                      Rollback
                    </button>
                  )}
                </div>
              ))
            )}
          </div>
        </TabsContent>

        {/* Environment */}
        <TabsContent value="environment" className="max-w-2xl pb-[46vh]">
          <EnvEditor app={project.name} />
        </TabsContent>

        {/* Settings */}
        <TabsContent value="settings" className="max-w-2xl pb-[46vh]">
          <SettingsForm
            name={project.name}
            branch={project.branch}
            rootDir={project.rootDir}
            port={project.port}
            auto={project.auto}
            onSaved={reload}
            onDeleted={() => navigate('/')}
            onRedeploy={() => term.redeploy(project.name)}
          />
        </TabsContent>
      </Tabs>

      {term.stream && <Drawer stream={term.stream} onClose={term.close} />}
    </div>
  )
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 py-2.5 text-sm first:pt-0 last:pb-0">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="truncate text-right">{children}</dd>
    </div>
  )
}

function SettingsForm({
  name,
  branch,
  rootDir,
  port,
  auto,
  onSaved,
  onDeleted,
  onRedeploy,
}: {
  name: string
  branch: string
  rootDir: string
  port: string
  auto: boolean
  onSaved: () => void
  onDeleted: () => void
  onRedeploy: () => void
}) {
  const [form, setForm] = useState({ branch, rootDir, port, auto })
  const [saved, setSaved] = useState(false)

  const save = async () => {
    await projectsService.update(name, form)
    setSaved(true)
    onSaved()
  }

  const del = async () => {
    if (!confirm('Delete ' + name + '? This stops the app and removes its config.')) return
    await projectsService.stop(name)
    onDeleted()
  }

  return (
    <div className="flex flex-col gap-6">
      <section className="rounded-xl border border-white/8 p-5">
        <h2 className="mb-4 text-sm font-semibold">Build &amp; deploy</h2>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Branch">
            <Input
              value={form.branch}
              placeholder="main"
              onChange={(e) => setForm((f) => ({ ...f, branch: e.target.value }))}
              className="font-mono"
            />
          </Field>
          <Field label="Root directory">
            <Input
              value={form.rootDir}
              placeholder="/"
              onChange={(e) => setForm((f) => ({ ...f, rootDir: e.target.value }))}
              className="font-mono"
            />
          </Field>
          <Field label="Port">
            <Input
              value={form.port}
              placeholder="3000"
              onChange={(e) => setForm((f) => ({ ...f, port: e.target.value }))}
              className="font-mono"
            />
          </Field>
          <Field label="Auto-deploy on push">
            <button
              type="button"
              onClick={() => setForm((f) => ({ ...f, auto: !f.auto }))}
              className={
                'h-9 rounded-md border px-3 font-mono text-xs uppercase transition-colors ' +
                (form.auto
                  ? 'border-emerald-400/30 bg-emerald-400/10 text-emerald-300'
                  : 'border-white/15 text-muted-foreground')
              }
            >
              {form.auto ? 'On' : 'Off'}
            </button>
          </Field>
        </div>
        <div className="mt-5 flex items-center justify-end gap-3">
          {saved && <span className="text-muted-foreground text-xs">Saved.</span>}
          <Button size="sm" variant="outline" onClick={onRedeploy}>
            <RotateCw />
            Redeploy
          </Button>
          <Button size="sm" onClick={save}>
            Save changes
          </Button>
        </div>
      </section>

      <section className="rounded-xl border border-rose-500/20 bg-rose-500/[0.03] p-5">
        <h2 className="mb-1 text-sm font-semibold text-rose-300">Danger zone</h2>
        <p className="text-muted-foreground mb-4 text-sm">
          Stop this app and remove its configuration. This cannot be undone.
        </p>
        <Button
          size="sm"
          variant="outline"
          onClick={del}
          className="border-rose-500/30 text-rose-300 hover:bg-rose-500/10 hover:text-rose-200"
        >
          Delete project
        </Button>
      </section>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-2">
      <Label className="text-muted-foreground text-xs">{label}</Label>
      {children}
    </div>
  )
}
