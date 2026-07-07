import { zodResolver } from '@hookform/resolvers/zod'
import { ChevronRight, Cog, ExternalLink, GitBranch, Play, Plus, RotateCcw, RotateCw, Trash2 } from 'lucide-react'
import { useState, type FormEvent, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { Link, useNavigate, useParams } from 'react-router'

import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { jobSchema, type JobInput } from '@/validations/job'
import { projectSettingsSchema, type ProjectSettingsInput } from '@/validations/project-settings'
import { workerSchema, type WorkerInput } from '@/validations/worker'
import { ConsoleTerminal } from '@/components/console-terminal'
import { Drawer } from '@/components/drawer'
import { EnvEditor } from '@/components/env-editor'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useConsole } from '@/hooks/use-console'
import { useProject } from '@/hooks/use-project'
import { errText } from '@/lib/errors'
import { useConfirm } from '@/providers/confirm-provider'
import { projectsService, type Job, type Preview, type Worker } from '@/services/api.service'
import { useQuery, useQueryClient } from '@tanstack/react-query'

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
  const confirm = useConfirm()

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
          <TabsTrigger value="workers">Workers</TabsTrigger>
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
                      onClick={async () => {
                        if (
                          await confirm({
                            title: `Roll back ${project.name}?`,
                            description: 'It re-runs this build instantly — no rebuild.',
                            confirmText: 'Roll back',
                          })
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

        <TabsContent value="workers" className="max-w-3xl pb-[46vh]">
          <WorkersPanel app={project.name} />
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
            running={project.running}
            release={project.release}
            autoscale={project.autoscale}
            scaleMin={project.scaleMin}
            scaleMax={project.scaleMax}
            scaleCpu={project.scaleCpu}
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
  const confirm = useConfirm()
  const { data: jobs = [] } = useQuery<Job[]>({
    queryKey: ['jobs', app],
    queryFn: () => projectsService.jobs(app),
  })
  const [adding, setAdding] = useState(false)
  const [busy, setBusy] = useState('')
  const [serverError, setServerError] = useState('')
  const [output, setOutput] = useState<{ id: string; ok: boolean; text: string } | null>(null)
  const reload = () => qc.invalidateQueries({ queryKey: ['jobs', app] })

  const form = useForm<JobInput>({
    resolver: zodResolver(jobSchema),
    defaultValues: { name: '', schedule: '0 3 * * *', command: '' },
  })

  const create = form.handleSubmit(async (values) => {
    setServerError('')
    try {
      await projectsService.createJob(app, values.name || 'job', values.schedule, values.command)
      form.reset()
      setAdding(false)
      reload()
    } catch (err) {
      setServerError(errText(err, 'Could not create the job.'))
    }
  })

  const run = async (id: string) => {
    setBusy(id)
    setOutput(null)
    setServerError('')
    try {
      const r = await projectsService.runJob(id)
      setOutput({ id, ok: r.ok, text: r.output?.trim() || (r.ok ? 'Done.' : 'Failed.') })
      reload()
    } catch (err) {
      setServerError(errText(err, 'Could not run the job.'))
    } finally {
      setBusy('')
    }
  }

  const del = async (id: string) => {
    if (!(await confirm({ title: 'Delete this job?', confirmText: 'Delete', destructive: true }))) return
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
          <FieldGroup className="gap-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="job-name">Name</FieldLabel>
                <Input id="job-name" placeholder="nightly cleanup" {...form.register('name')} />
              </Field>
              <Field data-invalid={!!form.formState.errors.schedule}>
                <FieldLabel htmlFor="job-schedule">Schedule (cron)</FieldLabel>
                <Input
                  id="job-schedule"
                  className="font-mono"
                  placeholder="0 3 * * *"
                  aria-invalid={!!form.formState.errors.schedule}
                  {...form.register('schedule')}
                />
                <FieldError errors={[form.formState.errors.schedule]} />
              </Field>
            </div>
            <Field data-invalid={!!form.formState.errors.command}>
              <FieldLabel htmlFor="job-command">Command</FieldLabel>
              <Input
                id="job-command"
                className="font-mono"
                placeholder="npm run cleanup"
                aria-invalid={!!form.formState.errors.command}
                {...form.register('command')}
              />
              <FieldError errors={[form.formState.errors.command]} />
            </Field>
          </FieldGroup>
          {serverError && <p className="text-xs text-rose-300">{serverError}</p>}
          <div className="flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={() => {
                setAdding(false)
                setServerError('')
                form.reset()
              }}
              className="text-muted-foreground hover:text-foreground text-xs"
            >
              Cancel
            </button>
            <Button size="sm" type="submit" disabled={form.formState.isSubmitting}>
              {form.formState.isSubmitting ? 'Adding…' : 'Add job'}
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
      {serverError && !adding && <p className="text-xs text-rose-300">{serverError}</p>}
    </div>
  )
}

function WorkersPanel({ app }: { app: string }) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const { data: workers = [] } = useQuery<Worker[]>({
    queryKey: ['workers', app],
    queryFn: () => projectsService.workers(app),
    refetchInterval: 8000,
  })
  const [adding, setAdding] = useState(false)
  const [serverError, setServerError] = useState('')
  const reload = () => qc.invalidateQueries({ queryKey: ['workers', app] })

  const form = useForm<WorkerInput>({
    resolver: zodResolver(workerSchema),
    defaultValues: { name: 'worker', command: '', replicas: 1 },
  })
  const replicas = form.watch('replicas')
  const setReplicas = (n: number) =>
    form.setValue('replicas', Math.min(10, Math.max(1, n)), { shouldDirty: true })

  const create = form.handleSubmit(async (values) => {
    setServerError('')
    try {
      await projectsService.setWorker(app, values.name, values.command, values.replicas)
      form.reset()
      setAdding(false)
      reload()
    } catch (err) {
      setServerError(errText(err, 'Could not save the worker.'))
    }
  })

  const del = async (id: string) => {
    if (!(await confirm({ title: 'Delete this worker?', confirmText: 'Delete', destructive: true }))) return
    await projectsService.deleteWorker(id)
    reload()
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-end justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">Background workers</h2>
          <p className="text-muted-foreground mt-1 text-xs">
            Long-running processes from this app's image with a different command and no HTTP port —
            recreated on every deploy. For scheduled one-offs, use Jobs.
          </p>
        </div>
        {!adding && (
          <Button size="sm" onClick={() => setAdding(true)} className="shrink-0">
            <Plus />
            New worker
          </Button>
        )}
      </div>

      {adding && (
        <form
          onSubmit={create}
          className="flex flex-col gap-3 rounded-xl border border-white/10 bg-linear-to-b from-white/2 to-transparent p-4"
        >
          <FieldGroup className="gap-3">
            <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
              <Field data-invalid={!!form.formState.errors.name}>
                <FieldLabel htmlFor="wk-name">Name</FieldLabel>
                <Input
                  id="wk-name"
                  className="font-mono"
                  placeholder="worker"
                  aria-invalid={!!form.formState.errors.name}
                  {...form.register('name')}
                />
                <FieldError errors={[form.formState.errors.name]} />
              </Field>
              <Field>
                <FieldLabel>Replicas</FieldLabel>
                <Stepper
                  value={replicas}
                  onDec={() => setReplicas(replicas - 1)}
                  onInc={() => setReplicas(replicas + 1)}
                />
              </Field>
            </div>
            <Field data-invalid={!!form.formState.errors.command}>
              <FieldLabel htmlFor="wk-command">Command</FieldLabel>
              <Input
                id="wk-command"
                className="font-mono"
                placeholder="node worker.js"
                aria-invalid={!!form.formState.errors.command}
                {...form.register('command')}
              />
              <FieldError errors={[form.formState.errors.command]} />
            </Field>
          </FieldGroup>
          {serverError && <p className="text-xs text-rose-300">{serverError}</p>}
          <div className="flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={() => {
                setAdding(false)
                setServerError('')
                form.reset()
              }}
              className="text-muted-foreground hover:text-foreground text-xs"
            >
              Cancel
            </button>
            <Button size="sm" type="submit" disabled={form.formState.isSubmitting}>
              {form.formState.isSubmitting ? 'Saving…' : 'Add worker'}
            </Button>
          </div>
        </form>
      )}

      {workers.length === 0 && !adding ? (
        <div className="text-muted-foreground rounded-xl border border-white/8 py-14 text-center text-sm">
          No workers yet.
        </div>
      ) : (
        workers.length > 0 && (
          <div className="divide-y divide-white/5 overflow-hidden rounded-xl border border-white/8">
            {workers.map((wk) => (
              <div key={wk.id} className="flex items-center gap-3 p-4">
                <Cog className="text-muted-foreground h-4 w-4 shrink-0" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{wk.name}</p>
                  <p className="text-muted-foreground mt-0.5 truncate font-mono text-xs">{wk.command}</p>
                </div>
                <span
                  className={
                    'shrink-0 font-mono text-[11px] tabular-nums ' +
                    (wk.running >= wk.replicas ? 'text-emerald-300' : 'text-amber-300')
                  }
                >
                  {wk.running}/{wk.replicas} running
                </span>
                <button
                  onClick={() => del(wk.id)}
                  title="Delete"
                  className="text-muted-foreground p-1.5 transition hover:text-rose-300"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        )
      )}
      {serverError && !adding && <p className="text-xs text-rose-300">{serverError}</p>}
    </div>
  )
}

function Stepper({
  value,
  onDec,
  onInc,
  min = 1,
  max = 10,
}: {
  value: number
  onDec: () => void
  onInc: () => void
  min?: number
  max?: number
}) {
  return (
    <div className="flex h-9 w-28 items-center justify-between rounded-md border border-white/15 px-1">
      <button
        type="button"
        onClick={onDec}
        disabled={value <= min}
        aria-label="Decrease"
        className="text-muted-foreground hover:text-foreground grid h-7 w-7 place-items-center rounded text-base disabled:opacity-30"
      >
        −
      </button>
      <span className="font-mono text-sm tabular-nums">{value}</span>
      <button
        type="button"
        onClick={onInc}
        disabled={value >= max}
        aria-label="Increase"
        className="text-muted-foreground hover:text-foreground grid h-7 w-7 place-items-center rounded text-base disabled:opacity-30"
      >
        +
      </button>
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
  running,
  release,
  autoscale,
  scaleMin,
  scaleMax,
  scaleCpu,
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
  running: number
  release: string
  autoscale: boolean
  scaleMin: number
  scaleMax: number
  scaleCpu: number
  onSaved: () => void
  onDeleted: () => void
  onRedeploy: () => void
}) {
  const [saved, setSaved] = useState(false)
  const confirm = useConfirm()
  const form = useForm<ProjectSettingsInput>({
    resolver: zodResolver(projectSettingsSchema),
    defaultValues: {
      branch,
      rootDir,
      port,
      auto,
      previewAuto,
      replicas: replicas || 1,
      release: release || '',
      autoscale,
      scaleMin: scaleMin || 1,
      scaleMax: scaleMax || Math.max(scaleMin || 1, replicas || 2),
      scaleCpu: scaleCpu || 70,
    },
  })
  const { register, handleSubmit, watch, setValue, formState } = form
  const values = watch()
  const setReplicas = (n: number) =>
    setValue('replicas', Math.min(10, Math.max(1, n)), { shouldDirty: true })
  const setNum = (field: 'scaleMin' | 'scaleMax', n: number) =>
    setValue(field, Math.min(10, Math.max(1, n)), { shouldDirty: true })
  const toggleClass = (on: boolean) =>
    'h-9 rounded-md border px-3 font-mono text-xs uppercase transition-colors ' +
    (on
      ? 'border-emerald-400/30 bg-emerald-400/10 text-emerald-300'
      : 'border-white/15 text-muted-foreground')

  const save = handleSubmit(async (data) => {
    await projectsService.update(name, data)
    setSaved(true)
    onSaved()
  })

  const del = async () => {
    if (
      !(await confirm({
        title: `Delete ${name}?`,
        description: 'This stops the app and removes its config.',
        confirmText: 'Delete',
        destructive: true,
      }))
    )
      return
    await projectsService.stop(name)
    onDeleted()
  }

  return (
    <div className="flex flex-col gap-6">
      <section className="rounded-xl border border-white/8 p-5">
        <h2 className="mb-4 text-sm font-semibold">Build &amp; deploy</h2>
        <FieldGroup className="grid gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="s-branch">Branch</FieldLabel>
            <Input id="s-branch" placeholder="main" className="font-mono" {...register('branch')} />
          </Field>
          <Field>
            <FieldLabel htmlFor="s-root">Root directory</FieldLabel>
            <Input id="s-root" placeholder="/" className="font-mono" {...register('rootDir')} />
          </Field>
          <Field data-invalid={!!formState.errors.port}>
            <FieldLabel htmlFor="s-port">Port</FieldLabel>
            <Input
              id="s-port"
              placeholder="3000"
              className="font-mono"
              aria-invalid={!!formState.errors.port}
              {...register('port')}
            />
            <FieldError errors={[formState.errors.port]} />
          </Field>
          <Field>
            <FieldLabel htmlFor="s-auto">Auto-deploy on push</FieldLabel>
            <button
              id="s-auto"
              type="button"
              onClick={() => setValue('auto', !values.auto, { shouldDirty: true })}
              className={toggleClass(values.auto)}
            >
              {values.auto ? 'On' : 'Off'}
            </button>
          </Field>
          <Field>
            <FieldLabel htmlFor="s-preview">Preview deployments</FieldLabel>
            <button
              id="s-preview"
              type="button"
              onClick={() => setValue('previewAuto', !values.previewAuto, { shouldDirty: true })}
              className={toggleClass(values.previewAuto)}
            >
              {values.previewAuto ? 'On' : 'Off'}
            </button>
          </Field>
          {!values.autoscale && (
            <Field>
              <FieldLabel>Replicas</FieldLabel>
              <Stepper
                value={values.replicas}
                onDec={() => setReplicas(values.replicas - 1)}
                onInc={() => setReplicas(values.replicas + 1)}
              />
            </Field>
          )}
        </FieldGroup>
        <p className="text-muted-foreground mt-3 text-xs">
          Replicas run identical copies of the app behind the router, sharing traffic. Preview
          deployments spin up an environment automatically for any push to a branch other than{' '}
          <span className="text-foreground/70 font-mono">{values.branch || 'main'}</span>.
        </p>

        <div className="mt-4 rounded-lg border border-white/8 p-4">
          <div className="flex items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-2">
                <h3 className="text-sm font-medium">Autoscaling</h3>
                {values.autoscale && (
                  <span className="text-muted-foreground rounded bg-white/5 px-1.5 py-0.5 font-mono text-[10px] tabular-nums">
                    {running} running
                  </span>
                )}
              </div>
              <p className="text-muted-foreground mt-0.5 text-xs">
                Add and retire replicas automatically to hold each one near a target CPU.
              </p>
            </div>
            <button
              type="button"
              onClick={() => setValue('autoscale', !values.autoscale, { shouldDirty: true })}
              className={toggleClass(values.autoscale)}
            >
              {values.autoscale ? 'On' : 'Off'}
            </button>
          </div>
          {values.autoscale && (
            <FieldGroup className="mt-4 grid gap-4 sm:grid-cols-3">
              <Field>
                <FieldLabel>Min replicas</FieldLabel>
                <Stepper
                  value={values.scaleMin}
                  onDec={() => setNum('scaleMin', values.scaleMin - 1)}
                  onInc={() => setNum('scaleMin', values.scaleMin + 1)}
                />
              </Field>
              <Field data-invalid={!!formState.errors.scaleMax}>
                <FieldLabel>Max replicas</FieldLabel>
                <Stepper
                  value={values.scaleMax}
                  onDec={() => setNum('scaleMax', values.scaleMax - 1)}
                  onInc={() => setNum('scaleMax', values.scaleMax + 1)}
                />
                <FieldError errors={[formState.errors.scaleMax]} />
              </Field>
              <Field data-invalid={!!formState.errors.scaleCpu}>
                <FieldLabel htmlFor="s-target">Target CPU %</FieldLabel>
                <Input
                  id="s-target"
                  type="number"
                  min={10}
                  max={100}
                  step={5}
                  className="font-mono"
                  aria-invalid={!!formState.errors.scaleCpu}
                  {...register('scaleCpu', { valueAsNumber: true })}
                />
                <FieldError errors={[formState.errors.scaleCpu]} />
              </Field>
            </FieldGroup>
          )}
        </div>
        <Field className="mt-4">
          <FieldLabel htmlFor="s-release">Release command</FieldLabel>
          <Input id="s-release" placeholder="e.g. npm run migrate" className="font-mono" {...register('release')} />
          <FieldDescription>
            Runs once in a one-off container before each new version goes live — a non-zero exit aborts
            the deploy, so the old version keeps serving.
          </FieldDescription>
        </Field>
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
  const confirm = useConfirm()
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
        onClick={async () => {
          if (
            await confirm({
              title: `Tear down ${host}?`,
              description: 'This removes the preview and its build.',
              confirmText: 'Tear down',
              destructive: true,
            })
          )
            onTeardown()
        }}
        title="Tear down preview"
        className="text-muted-foreground shrink-0 p-1 opacity-0 transition hover:text-rose-300 group-hover:opacity-100"
      >
        <Trash2 className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}
