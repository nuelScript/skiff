import { StepsEditor, stepsLine } from './job-steps'
import type { JobStep } from '@/services/api.service'
import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Controller, useForm } from 'react-hook-form'
import { History, Loader2, Play, Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { queryKeys } from '@/constants/query-keys'
import { relTime } from '@/lib/format'
import { errText } from '@/lib/errors'
import { jobSettingsSummary, jobTriggers } from '@/lib/jobs'
import { JobWebhook } from '@/pages/project/job-webhook'
import { useConfirm } from '@/providers/confirm-provider'
import { projectsService, storageService, type Job } from '@/services/api.service'
import { jobSchema, type JobInput } from '@/validations/job'
import { JobHistory } from '@/pages/project/job-history'

export function JobsPanel({ app, cli }: { app: string; cli: boolean }) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const { data: jobs = [] } = useQuery<Job[]>({
    queryKey: queryKeys.jobs(app),
    queryFn: () => projectsService.jobs(app),
    refetchInterval: (q) => (q.state.data?.some((j) => j.running) ? 3000 : false),
  })
  const [adding, setAdding] = useState(false)
  const [history, setHistory] = useState('')
  const [busy, setBusy] = useState('')
  const [serverError, setServerError] = useState('')
  const [output, setOutput] = useState<{ id: string; ok: boolean; text: string } | null>(null)
  const reload = () => qc.invalidateQueries({ queryKey: queryKeys.jobs(app) })

  const form = useForm<JobInput>({
    resolver: zodResolver(jobSchema),
    defaultValues: {
      name: '',
      trigger: 'schedule',
      schedule: '0 3 * * *',
      bucket: '',
      prefix: '',
      command: '',
      usesSteps: false,
      timeoutMinutes: '',
      retries: '',
      catchUp: '',
    },
  })
  const trigger = form.watch('trigger')
  const usesSteps = form.watch('usesSteps')
  const [steps, setSteps] = useState<JobStep[]>([
    { name: 'extract', command: '', after: [] },
    { name: 'load', command: '', after: ['extract'] },
  ])
  const { data: buckets = [] } = useQuery({
    queryKey: queryKeys.storage,
    queryFn: () => storageService.list(),
    enabled: adding && trigger === 'bucket',
  })

  const create = form.handleSubmit(async (values) => {
    setServerError('')
    try {
      await projectsService.createJob(app, {
        name: values.name || 'job',
        schedule: values.trigger === 'schedule' ? values.schedule : '',
        webhook: values.trigger === 'webhook',
        bucket: values.trigger === 'bucket' ? values.bucket : '',
        prefix: values.trigger === 'bucket' ? values.prefix : '',
        command: values.usesSteps ? '' : values.command,
        steps: values.usesSteps ? steps.map((s) => ({ ...s, name: s.name.trim(), command: s.command.trim() })) : undefined,
        timeout: values.timeoutMinutes ? +values.timeoutMinutes * 60 : 0,
        retries: values.retries ? +values.retries : 0,
        catchUp: values.catchUp,
      })
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
      qc.invalidateQueries({ queryKey: queryKeys.jobRuns(id) })
    } catch (err) {
      setServerError(errText(err, 'Could not run the job.'))
    } finally {
      setBusy('')
    }
  }

  const del = async (id: string) => {
    if (!(await confirm({ title: 'Delete this job?', confirmText: 'Delete', destructive: true })))
      return
    await projectsService.deleteJob(id)
    reload()
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-end justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">Jobs</h2>
          <p className="text-muted-foreground mt-1 text-xs">
            {cli
              ? "Commands in one-off containers with this app's environment. They come from the [[jobs]] section of its skiff.toml."
              : "Run a command in a one-off container with this app's environment: on a schedule, when a webhook is called, or for each new file in a bucket."}
          </p>
        </div>
        {!cli && !adding && (
          <Button size="sm" onClick={() => setAdding(true)} className="shrink-0">
            <Plus />
            New job
          </Button>
        )}
      </div>

      {adding && (
        <form
          onSubmit={create}
          className="flex flex-col gap-3 rounded-xl border border-foreground/10 bg-linear-to-b from-foreground/2 to-transparent p-4"
        >
          <FieldGroup className="gap-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="job-name">Name</FieldLabel>
                <Input id="job-name" placeholder="nightly cleanup" {...form.register('name')} />
              </Field>
              <Field>
                <FieldLabel htmlFor="job-trigger">Runs</FieldLabel>
                <Controller
                  control={form.control}
                  name="trigger"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="job-trigger" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="schedule">On a schedule</SelectItem>
                        <SelectItem value="webhook">When its webhook is called</SelectItem>
                        <SelectItem value="bucket">For each new file in a bucket</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
              </Field>
            </div>
            {trigger === 'schedule' && (
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
            )}
            {trigger === 'webhook' && (
              <FieldDescription>
                Once it's added, the job shows its webhook address and the secret to sign requests with.
              </FieldDescription>
            )}
            {trigger === 'bucket' && (
              <div className="grid gap-3 sm:grid-cols-2">
                <Field data-invalid={!!form.formState.errors.bucket}>
                  <FieldLabel htmlFor="job-bucket">Bucket</FieldLabel>
                  <Controller
                    control={form.control}
                    name="bucket"
                    render={({ field }) => (
                      <Select value={field.value} onValueChange={field.onChange} disabled={buckets.length === 0}>
                        <SelectTrigger id="job-bucket" className="w-full" aria-invalid={!!form.formState.errors.bucket}>
                          <SelectValue placeholder={buckets.length ? 'Choose a bucket' : 'No buckets yet'} />
                        </SelectTrigger>
                        <SelectContent>
                          {buckets.map((b) => (
                            <SelectItem key={b.id} value={b.name}>
                              {b.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  />
                  <FieldError errors={[form.formState.errors.bucket]} />
                  {buckets.length === 0 && (
                    <FieldDescription>Create a bucket under Storage first.</FieldDescription>
                  )}
                </Field>
                <Field data-invalid={!!form.formState.errors.prefix}>
                  <FieldLabel htmlFor="job-prefix">Folder (optional)</FieldLabel>
                  <Input
                    id="job-prefix"
                    className="font-mono"
                    placeholder="incoming/"
                    aria-invalid={!!form.formState.errors.prefix}
                    {...form.register('prefix')}
                  />
                  <FieldError errors={[form.formState.errors.prefix]} />
                </Field>
                <FieldDescription className="sm:col-span-2">
                  Files already there are left alone. Each new one starts a run, with its key in{' '}
                  <code className="font-mono">SKIFF_OBJECT_KEY</code>. Skiff looks every 30 seconds.
                </FieldDescription>
              </div>
            )}
            <Field data-invalid={!!form.formState.errors.command}>
              <div className="flex items-center justify-between gap-3">
                <FieldLabel htmlFor="job-command">{usesSteps ? 'Steps' : 'Command'}</FieldLabel>
                <button
                  type="button"
                  onClick={() => form.setValue('usesSteps', !usesSteps)}
                  className="text-muted-foreground hover:text-foreground cursor-pointer text-xs transition-colors"
                >
                  {usesSteps ? 'One command instead' : 'Several steps instead'}
                </button>
              </div>
              {usesSteps ? (
                <>
                  <StepsEditor steps={steps} onChange={setSteps} />
                  <FieldDescription>
                    Each step runs in its own container once the steps it comes after have succeeded; steps with nothing between
                    them run side by side. A failed step stops only what comes after it. Steps share the app's volumes, not files
                    in their containers.
                  </FieldDescription>
                </>
              ) : (
                <Input
                  id="job-command"
                  className="font-mono"
                  placeholder="npm run cleanup"
                  aria-invalid={!!form.formState.errors.command}
                  {...form.register('command')}
                />
              )}
              <FieldError errors={[form.formState.errors.command]} />
            </Field>
            <div className={'grid gap-3 ' + (trigger === 'schedule' ? 'sm:grid-cols-3' : 'sm:grid-cols-2')}>
              <Field data-invalid={!!form.formState.errors.timeoutMinutes}>
                <FieldLabel htmlFor="job-timeout">Timeout (minutes)</FieldLabel>
                <Input
                  id="job-timeout"
                  inputMode="numeric"
                  placeholder="30"
                  aria-invalid={!!form.formState.errors.timeoutMinutes}
                  {...form.register('timeoutMinutes')}
                />
                <FieldError errors={[form.formState.errors.timeoutMinutes]} />
              </Field>
              <Field data-invalid={!!form.formState.errors.retries}>
                <FieldLabel htmlFor="job-retries">Retries</FieldLabel>
                <Input
                  id="job-retries"
                  inputMode="numeric"
                  placeholder="0"
                  aria-invalid={!!form.formState.errors.retries}
                  {...form.register('retries')}
                />
                <FieldError errors={[form.formState.errors.retries]} />
              </Field>
              {trigger === 'schedule' && (
                <Field>
                  <FieldLabel htmlFor="job-catchup">After downtime</FieldLabel>
                  <Controller
                    control={form.control}
                    name="catchUp"
                    render={({ field }) => (
                      <Select value={field.value || 'once'} onValueChange={(v) => field.onChange(v === 'skip' ? 'skip' : '')}>
                        <SelectTrigger id="job-catchup" className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="once">Run once</SelectItem>
                          <SelectItem value="skip">Skip missed runs</SelectItem>
                        </SelectContent>
                      </Select>
                    )}
                  />
                </Field>
              )}
            </div>
            <FieldDescription>
              A failed run is tried again after 30 seconds, then a minute, then two, up to ten minutes apart. Your
              alert channels hear when a job starts failing and when it works again.
            </FieldDescription>
          </FieldGroup>
          {serverError && <p className="text-xs text-danger">{serverError}</p>}
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
            <Button size="sm" type="submit" loading={form.formState.isSubmitting}>
              Add job
            </Button>
          </div>
        </form>
      )}

      {jobs.length === 0 && !adding ? (
        <div className="text-muted-foreground rounded-xl border border-foreground/8 py-14 text-center text-sm">
          No jobs yet.
        </div>
      ) : (
        jobs.length > 0 && (
          <div className="divide-y divide-foreground/5 overflow-hidden rounded-xl border border-foreground/8">
            {jobs.map((j) => (
              <div key={j.id} className="p-4">
                <div className="flex items-center gap-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{j.name}</p>
                    <p className="text-muted-foreground mt-0.5 truncate font-mono text-xs">
                      {j.steps?.length ? stepsLine(j.steps) : j.command}
                    </p>
                  </div>
                  {j.schedule && (
                    <span className="text-muted-foreground shrink-0 font-mono text-[11px]">{j.schedule}</span>
                  )}
                  <button
                    onClick={() => setHistory(history === j.id ? '' : j.id)}
                    title="Run history"
                    aria-label={`Run history of ${j.name}`}
                    aria-expanded={history === j.id}
                    className={
                      'cursor-pointer p-1.5 transition-colors ' +
                      (history === j.id ? 'text-foreground' : 'text-muted-foreground hover:text-foreground')
                    }
                  >
                    <History className="h-3.5 w-3.5" />
                  </button>
                  <button
                    onClick={() => run(j.id)}
                    disabled={busy === j.id || j.running}
                    title={j.running ? 'Already running' : 'Run now'}
                    aria-label={`Run ${j.name} now`}
                    className="text-muted-foreground hover:text-foreground cursor-pointer p-1.5 disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    {busy === j.id || j.running ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <Play className="h-3.5 w-3.5" />
                    )}
                  </button>
                  {!cli && (
                    <button
                      onClick={() => del(j.id)}
                      title="Delete"
                      aria-label={`Delete ${j.name}`}
                      className="text-muted-foreground cursor-pointer p-1.5 transition hover:text-danger"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  )}
                </div>
                <div className="text-muted-foreground mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[11px]">
                  {j.schedule && <span>next {fmtWhen(j.next)}</span>}
                  <span>
                    last run{' '}
                    {j.running ? (
                      <span className="text-foreground">running now</span>
                    ) : j.lastRun ? (
                      <>
                        {relTime(j.lastRun)} ·{' '}
                        <span className={j.lastOk ? 'text-success' : 'text-danger'}>
                          {j.lastOk ? 'ok' : 'failed'}
                        </span>
                      </>
                    ) : (
                      'never'
                    )}
                  </span>
                  {jobTriggers(j) && <span>runs {jobTriggers(j)}</span>}
                  {j.queued > 0 && <span>{j.queued} waiting</span>}
                  <span>{jobSettingsSummary(j)}</span>
                </div>
                {j.webhook && j.webhookSecret && <JobWebhook url={j.webhook} secret={j.webhookSecret} />}
                {history === j.id && (
                  <div className="mt-3">
                    <JobHistory id={j.id} live={j.running} />
                  </div>
                )}
                {output?.id === j.id && history !== j.id && (
                  <pre
                    className={
                      'mt-2 max-h-40 overflow-auto rounded-md border p-2.5 font-mono text-[11px] whitespace-pre-wrap ' +
                      (output.ok
                        ? 'border-foreground/8 bg-inset'
                        : 'border-danger/25 bg-danger/5 text-danger')
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
      {serverError && !adding && <p className="text-xs text-danger">{serverError}</p>}
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
