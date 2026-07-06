import { useState, type ReactNode, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useParams, useNavigate, Link } from 'react-router'
import { ChevronRight, ExternalLink, RotateCw, RotateCcw, GitBranch, Plus, Trash2, Play } from 'lucide-react'
import { useProject } from '@/hooks/use-project'
import { useConsole } from '@/hooks/use-console'
import { projectsService, type Preview, type Job } from '@/services/api.service'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EnvEditor } from '@/components/env-editor'
import { ConsoleTerminal } from '@/components/console-terminal'
import { Drawer } from '@/components/drawer'
import { errText } from '@/lib/errors'

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
      : status === 'canceled'
        ? 'bg-white/25'
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
          <TabsTrigger value="previews">
            Previews
            {project.previews.length > 0 && (
              <span className="text-muted-foreground ml-1.5 rounded-full bg-white/8 px-1.5 py-0.5 font-mono text-[10px] tabular-nums">
                {project.previews.length}
              </span>
            )}
          </TabsTrigger>
          <TabsTrigger value="console">Console</TabsTrigger>
          <TabsTrigger value="jobs">Jobs</TabsTrigger>
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

        {/* Previews */}
        <TabsContent value="previews" className="pb-[46vh]">
          <PreviewsPanel
            project={project.name}
            previews={project.previews}
            onCreate={(branch) => term.preview(project.name, branch)}
            onTeardown={async (name) => {
              await projectsService.stop(name)
              reload()
            }}
          />
        </TabsContent>

        {/* Console */}
        <TabsContent value="console" className="pb-8">
          <div className="mb-3">
            <h2 className="text-sm font-semibold">Console</h2>
            <p className="text-muted-foreground mt-0.5 text-xs">
              A shell inside the running container — run migrations, inspect state, debug.
            </p>
          </div>
          {project.state === 'running' ? (
            <div className="h-[60vh] overflow-hidden rounded-xl border border-white/10 bg-[#09090b] p-3">
              <ConsoleTerminal app={project.name} />
            </div>
          ) : (
            <p className="text-muted-foreground rounded-xl border border-white/8 py-16 text-center text-sm">
              The app isn't running — deploy it to open a console.
            </p>
          )}
        </TabsContent>

        {/* Environment */}
        <TabsContent value="environment" className="max-w-2xl pb-[46vh]">
          <EnvEditor app={project.name} />
        </TabsContent>

        {/* Jobs */}
        <TabsContent value="jobs" className="max-w-3xl pb-[46vh]">
          <JobsPanel app={project.name} />
        </TabsContent>

        {/* Settings */}
        <TabsContent value="settings" className="max-w-2xl pb-[46vh]">
          <SettingsForm
            name={project.name}
            branch={project.branch}
            rootDir={project.rootDir}
            port={project.port}
            auto={project.auto}
            previewAuto={project.previewAuto}
            replicas={project.replicas}
            release={project.release}
            onSaved={reload}
            onDeleted={() => navigate('/')}
            onRedeploy={() => term.redeploy(project.name)}
          />
        </TabsContent>
      </Tabs>

      {term.stream && <Drawer stream={term.stream} onClose={term.close} onStop={term.stop} />}
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

function fmtWhen(unix: number): string {
  if (!unix) return '—'
  return new Date(unix * 1000).toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

function JobsPanel({ app }: { app: string }) {
  const qc = useQueryClient()
  const { data: jobs = [] } = useQuery<Job[]>({
    queryKey: ['jobs', app],
    queryFn: () => projectsService.jobs(app),
  })
  const [adding, setAdding] = useState(false)
  const [name, setName] = useState('')
  const [schedule, setSchedule] = useState('0 3 * * *')
  const [command, setCommand] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  const [output, setOutput] = useState<{ id: string; ok: boolean; text: string } | null>(null)
  const reload = () => qc.invalidateQueries({ queryKey: ['jobs', app] })

  const create = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (!command.trim()) return
    setBusy('create')
    try {
      await projectsService.createJob(app, name.trim() || 'job', schedule.trim(), command.trim())
      setName('')
      setCommand('')
      setSchedule('0 3 * * *')
      setAdding(false)
      reload()
    } catch (err) {
      setError(errText(err, 'Could not create the job.'))
    } finally {
      setBusy('')
    }
  }

  const run = async (id: string) => {
    setBusy(id)
    setOutput(null)
    setError('')
    try {
      const r = await projectsService.runJob(id)
      setOutput({ id, ok: r.ok, text: r.output?.trim() || (r.ok ? 'Done.' : 'Failed.') })
      reload()
    } catch (err) {
      setError(errText(err, 'Could not run the job.'))
    } finally {
      setBusy('')
    }
  }

  const del = async (id: string) => {
    if (!confirm('Delete this job?')) return
    await projectsService.deleteJob(id)
    reload()
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-end justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">Scheduled jobs</h2>
          <p className="text-muted-foreground mt-1 text-xs">
            Run a command on a cron schedule in a one-off container, with this app's environment.
          </p>
        </div>
        {!adding && (
          <Button size="sm" onClick={() => setAdding(true)} className="shrink-0">
            <Plus />
            New job
          </Button>
        )}
      </div>

      {adding && (
        <form
          onSubmit={create}
          className="flex flex-col gap-3 rounded-xl border border-white/10 bg-linear-to-b from-white/2 to-transparent p-4"
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <label className="text-muted-foreground mb-1.5 block text-xs">Name</label>
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="nightly cleanup" />
            </div>
            <div>
              <label className="text-muted-foreground mb-1.5 block text-xs">Schedule (cron)</label>
              <Input
                value={schedule}
                onChange={(e) => {
                  setSchedule(e.target.value)
                  setError('')
                }}
                className="font-mono"
                placeholder="0 3 * * *"
              />
            </div>
          </div>
          <div>
            <label className="text-muted-foreground mb-1.5 block text-xs">Command</label>
            <Input
              value={command}
              onChange={(e) => {
                setCommand(e.target.value)
                setError('')
              }}
              className="font-mono"
              placeholder="npm run cleanup"
            />
          </div>
          {error && <p className="text-xs text-rose-300">{error}</p>}
          <div className="flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={() => {
                setAdding(false)
                setError('')
              }}
              className="text-muted-foreground hover:text-foreground text-xs"
            >
              Cancel
            </button>
            <Button size="sm" type="submit" disabled={busy === 'create' || !command.trim()}>
              {busy === 'create' ? 'Adding…' : 'Add job'}
            </Button>
          </div>
        </form>
      )}

      {jobs.length === 0 && !adding ? (
        <div className="text-muted-foreground rounded-xl border border-white/8 py-14 text-center text-sm">
          No scheduled jobs yet.
        </div>
      ) : (
        jobs.length > 0 && (
          <div className="divide-y divide-white/5 overflow-hidden rounded-xl border border-white/8">
            {jobs.map((j) => (
              <div key={j.id} className="p-4">
                <div className="flex items-center gap-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{j.name}</p>
                    <p className="text-muted-foreground mt-0.5 truncate font-mono text-xs">{j.command}</p>
                  </div>
                  <span className="text-muted-foreground shrink-0 font-mono text-[11px]">{j.schedule}</span>
                  <button
                    onClick={() => run(j.id)}
                    disabled={busy === j.id}
                    title="Run now"
                    className="text-muted-foreground hover:text-foreground p-1.5 disabled:opacity-40"
                  >
                    <Play className="h-3.5 w-3.5" />
                  </button>
                  <button
                    onClick={() => del(j.id)}
                    title="Delete"
                    className="text-muted-foreground p-1.5 transition hover:text-rose-300"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
                <div className="text-muted-foreground mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[11px]">
                  <span>next {fmtWhen(j.next)}</span>
                  <span>
                    last run{' '}
                    {j.lastRun ? (
                      <>
                        {rel(j.lastRun)} ·{' '}
                        <span className={j.lastOk ? 'text-emerald-300' : 'text-rose-300'}>
                          {j.lastOk ? 'ok' : 'failed'}
                        </span>
                      </>
                    ) : (
                      'never'
                    )}
                  </span>
                </div>
                {output?.id === j.id && (
                  <pre
                    className={
                      'mt-2 max-h-40 overflow-auto rounded-md border p-2.5 font-mono text-[11px] whitespace-pre-wrap ' +
                      (output.ok
                        ? 'border-white/8 bg-black/30'
                        : 'border-rose-500/25 bg-rose-500/5 text-rose-200')
                    }
                  >
                    {output.text}
                  </pre>
                )}
              </div>
            ))}
          </div>
        )
      )}
      {error && !adding && <p className="text-xs text-rose-300">{error}</p>}
    </div>
  )
}

function SettingsForm({
  name,
  branch,
  rootDir,
  port,
  auto,
  previewAuto,
  replicas,
  release,
  onSaved,
  onDeleted,
  onRedeploy,
}: {
  name: string
  branch: string
  rootDir: string
  port: string
  auto: boolean
  previewAuto: boolean
  replicas: number
  release: string
  onSaved: () => void
  onDeleted: () => void
  onRedeploy: () => void
}) {
  const [form, setForm] = useState({ branch, rootDir, port, auto, previewAuto, replicas: replicas || 1, release: release || '' })
  const [saved, setSaved] = useState(false)
  const setReplicas = (n: number) => setForm((f) => ({ ...f, replicas: Math.min(10, Math.max(1, n)) }))

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
          <Field label="Preview deployments">
            <button
              type="button"
              onClick={() => setForm((f) => ({ ...f, previewAuto: !f.previewAuto }))}
              className={
                'h-9 rounded-md border px-3 font-mono text-xs uppercase transition-colors ' +
                (form.previewAuto
                  ? 'border-emerald-400/30 bg-emerald-400/10 text-emerald-300'
                  : 'border-white/15 text-muted-foreground')
              }
            >
              {form.previewAuto ? 'On' : 'Off'}
            </button>
          </Field>
          <Field label="Replicas">
            <div className="flex h-9 w-28 items-center justify-between rounded-md border border-white/15 px-1">
              <button
                type="button"
                onClick={() => setReplicas(form.replicas - 1)}
                disabled={form.replicas <= 1}
                aria-label="Fewer replicas"
                className="text-muted-foreground hover:text-foreground grid h-7 w-7 place-items-center rounded text-base disabled:opacity-30"
              >
                −
              </button>
              <span className="font-mono text-sm tabular-nums">{form.replicas}</span>
              <button
                type="button"
                onClick={() => setReplicas(form.replicas + 1)}
                disabled={form.replicas >= 10}
                aria-label="More replicas"
                className="text-muted-foreground hover:text-foreground grid h-7 w-7 place-items-center rounded text-base disabled:opacity-30"
              >
                +
              </button>
            </div>
          </Field>
        </div>
        <p className="text-muted-foreground mt-3 text-xs">
          Replicas run identical copies of the app behind the router, sharing traffic. Preview
          deployments spin up an environment automatically for any push to a branch other than{' '}
          <span className="text-foreground/70 font-mono">{form.branch || 'main'}</span>.
        </p>
        <div className="mt-4">
          <label className="text-muted-foreground mb-1.5 block text-xs">Release command</label>
          <Input
            value={form.release}
            placeholder="e.g. npm run migrate"
            onChange={(e) => setForm((f) => ({ ...f, release: e.target.value }))}
            className="font-mono"
          />
          <p className="text-muted-foreground mt-1.5 text-xs">
            Runs once in a one-off container before each new version goes live — a non-zero exit aborts
            the deploy, so the old version keeps serving.
          </p>
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

function PreviewsPanel({
  project,
  previews,
  onCreate,
  onTeardown,
}: {
  project: string
  previews: Preview[]
  onCreate: (branch: string) => void
  onTeardown: (name: string) => void | Promise<void>
}) {
  const [branch, setBranch] = useState('')
  const create = (e: FormEvent) => {
    e.preventDefault()
    const b = branch.trim()
    if (!b) return
    onCreate(b)
    setBranch('')
  }
  return (
    <div className="max-w-3xl space-y-4">
      <form
        onSubmit={create}
        className="rounded-xl border border-white/10 bg-linear-to-b from-white/3 to-transparent p-4"
      >
        <h2 className="text-sm font-medium">New preview environment</h2>
        <p className="text-muted-foreground mt-1 mb-3 text-xs">
          Deploy any branch of <span className="text-foreground/70 font-mono">{project}</span> to its
          own live URL with its own certificate. Pushes to the branch redeploy it automatically.
        </p>
        <div className="flex flex-col gap-2 sm:flex-row">
          <div className="relative min-w-0 flex-1">
            <GitBranch className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2" />
            <input
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
              placeholder="branch name — e.g. feat/login"
              className="h-9 w-full rounded-[6px] border border-white/12 bg-black/30 pr-3 pl-9 font-mono text-sm outline-none placeholder:text-white/25 focus-visible:border-white/30"
            />
          </div>
          <Button type="submit" size="sm" disabled={!branch.trim()} className="shrink-0">
            <Plus className="h-4 w-4" />
            Create preview
          </Button>
        </div>
      </form>

      {previews.length === 0 ? (
        <div className="text-muted-foreground flex flex-col items-center gap-2 rounded-xl border border-white/8 py-12 text-sm">
          <GitBranch className="h-5 w-5 opacity-40" />
          <span>No preview environments yet.</span>
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-white/8">
          {previews.map((pv) => (
            <PreviewRow key={pv.name} pv={pv} onTeardown={() => onTeardown(pv.name)} />
          ))}
        </div>
      )}
    </div>
  )
}

function PreviewRow({ pv, onTeardown }: { pv: Preview; onTeardown: () => void }) {
  const host = pv.url.replace(/^https?:\/\//, '')
  const label = pv.state === 'running' ? 'Ready' : pv.status || pv.state
  return (
    <div className="group flex items-center gap-3.5 border-b border-white/5 px-4 py-3 last:border-0 hover:bg-white/2">
      <span className={'h-2 w-2 shrink-0 rounded-full ' + deployDot(pv.status)} />
      <div className="min-w-0 flex-1">
        <a
          href={pv.url}
          target="_blank"
          rel="noreferrer"
          className="hover:text-foreground group/link flex items-center gap-1.5 text-sm font-medium"
        >
          <span className="truncate font-mono">{host}</span>
          <ExternalLink className="h-3 w-3 shrink-0 opacity-0 transition group-hover/link:opacity-60" />
        </a>
        <p className="text-muted-foreground mt-0.5 flex items-center gap-1 truncate text-xs">
          <GitBranch className="h-3 w-3 shrink-0" /> {pv.branch}
        </p>
      </div>
      <span className="text-muted-foreground shrink-0 text-[11px] capitalize">{label}</span>
      <button
        onClick={() => {
          if (confirm(`Tear down ${host}? This removes the preview and its build.`)) onTeardown()
        }}
        title="Tear down preview"
        className="text-muted-foreground shrink-0 p-1 opacity-0 transition hover:text-rose-300 group-hover:opacity-100"
      >
        <Trash2 className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}
