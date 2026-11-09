import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Play, Square } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { relTime } from '@/lib/format'
import { useConfirm } from '@/providers/confirm-provider'
import { aiService, type AgentEvent, type Sandbox } from '@/services/api.service'
import { JobWebhook } from './job-webhook'

const sourceLabel: Record<string, string> = { webhook: 'Webhook', github: 'GitHub', schedule: 'Schedule' }

// AgentPanel shows an agent's state, its inbox and how work reaches it, with one button that stops everything it does.
export function AgentPanel({ app }: { app: string }) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const key = queryKeys.agent(app)
  const { data } = useQuery({ queryKey: key, queryFn: () => aiService.agent(app), refetchInterval: 10000 })
  const [schedule, setSchedule] = useState<string | null>(null)
  const reload = () => {
    qc.invalidateQueries({ queryKey: key })
    qc.invalidateQueries({ queryKey: queryKeys.projectAI(app) })
  }

  const power = useMutation({ mutationFn: (action: 'stop' | 'start') => aiService.agentAction(app, action), onSuccess: reload })
  const save = useMutation({
    mutationFn: (s: string) => aiService.setAgentSchedule(app, s),
    onSuccess: () => {
      setSchedule(null)
      reload()
    },
  })

  if (!data?.agent) return null

  const stop = async () => {
    if (
      await confirm({
        title: `Stop ${app}?`,
        description:
          'Its containers and any commands it is running stop now, and Skiff refuses its model calls and holds its inbox until you start it again.',
        confirmText: 'Stop agent',
        destructive: true,
      })
    )
      power.mutate('stop')
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (schedule !== null) save.mutate(schedule.trim())
  }

  const error = power.error ?? save.error
  return (
    <section className="mb-7 flex flex-col gap-4 rounded-xl border border-foreground/8 p-4 sm:p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-2 text-sm font-semibold">
            Agent
            {data.stopped && (
              <span className="rounded bg-danger/10 px-1.5 py-0.5 font-mono text-[10px] text-danger uppercase">
                stopped
              </span>
            )}
          </h2>
          <p className="text-muted-foreground mt-1 max-w-2xl text-xs">
            No public address. Its files in <code className="font-mono">{data.statePath}</code> survive deploys and
            restarts, and work reaches it through its inbox: it asks for the next event, and an event it doesn't
            finish within five minutes, as when it crashes, is handed out again.
          </p>
        </div>
        {data.stopped ? (
          <Button size="sm" variant="outline" loading={power.isPending} onClick={() => power.mutate('start')}>
            <Play className="h-3.5 w-3.5" />
            Start
          </Button>
        ) : (
          <Button size="sm" variant="destructive" loading={power.isPending} onClick={() => void stop()}>
            <Square className="h-3.5 w-3.5" />
            Stop agent
          </Button>
        )}
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="flex flex-col gap-3">
          <h3 className="text-xs font-medium">Work comes from</h3>
          <JobWebhook
            url={data.webhook}
            secret={data.webhookSecret}
            after={
<>Each request becomes one event, and a GitHub event arrives with its type.</>
            }
          />
          <form onSubmit={submit} className="flex flex-wrap items-center gap-2">
            <label htmlFor="agent-schedule" className="text-muted-foreground text-xs">
              On a schedule
            </label>
            <Input
              id="agent-schedule"
              className="h-8 w-40 font-mono text-xs"
              placeholder="*/15 * * * *"
              value={schedule ?? data.schedule}
              onChange={(e) => setSchedule(e.target.value)}
            />
            <Button size="sm" type="submit" variant="outline" loading={save.isPending} disabled={schedule === null}>
              Save
            </Button>
            {data.next > 0 && schedule === null && (
              <span className="text-muted-foreground text-xs">next {new Date(data.next * 1000).toLocaleString()}</span>
            )}
          </form>
        </div>

        <div className="flex min-w-0 flex-col gap-3">
          <h3 className="text-xs font-medium">
            Inbox{' '}
            <span className="text-muted-foreground font-normal">
              · {data.pending} waiting
            </span>
          </h3>
          {data.events.length === 0 ? (
            <p className="text-muted-foreground rounded-lg border border-dashed border-foreground/10 p-4 text-xs">
              Nothing yet. Events from the webhook and the schedule show here.
            </p>
          ) : (
            <ul className="max-h-64 divide-y divide-foreground/6 overflow-auto rounded-lg border border-foreground/8">
              {data.events.map((e) => (
                <EventRow key={e.id} e={e} />
              ))}
            </ul>
          )}
        </div>
      </div>

      {data.sandboxes.length > 0 && (
        <div className="flex min-w-0 flex-col gap-3">
          <h3 className="text-xs font-medium">Sandboxes</h3>
          <ul className="divide-y divide-foreground/6 overflow-hidden rounded-lg border border-foreground/8">
            {data.sandboxes.map((s) => (
              <SandboxRow key={s.id} s={s} />
            ))}
          </ul>
        </div>
      )}

      <pre className="overflow-x-auto rounded-lg bg-inset p-3 font-mono text-[11px] leading-relaxed">
        {`# the agent's loop
r = requests.get(os.environ["SKIFF_AI_URL"] + "/_skiff/events?wait=60",
                 headers={"Authorization": "Bearer " + os.environ["SKIFF_AI_TOKEN"]})
if r.status_code == 200:
    event = r.json()          # id, source, kind, body
    handle(event)
    requests.post(f"{os.environ['SKIFF_AI_URL']}/_skiff/events/{event['id']}/done",
                  headers={"Authorization": "Bearer " + os.environ["SKIFF_AI_TOKEN"]})`}
      </pre>
      {error && <p className="text-xs text-danger">{errText(error, 'Something went wrong.')}</p>}
    </section>
  )
}

function EventRow({ e }: { e: AgentEvent }) {
  const state = e.done ? 'done' : e.deliveries > 0 ? 'working' : 'waiting'
  return (
    <li className="flex min-w-0 items-start justify-between gap-3 px-3 py-2 text-xs">
      <div className="min-w-0">
        <p>
          {sourceLabel[e.source] ?? e.source}
          {e.kind && <span className="text-muted-foreground font-mono"> · {e.kind}</span>}
        </p>
        {e.body && <p className="text-muted-foreground truncate font-mono text-[11px]">{e.body}</p>}
      </div>
      <div className="shrink-0 text-right">
        <p className={state === 'done' ? 'text-muted-foreground' : state === 'working' ? 'text-warning' : ''}>
          {state}
          {e.deliveries > 1 && ` · try ${e.deliveries}`}
        </p>
        <p className="text-muted-foreground">{relTime(e.created)}</p>
      </div>
    </li>
  )
}

const sandboxStatus: Record<Sandbox['status'], string> = {
  running: 'running',
  ok: 'exited 0',
  failed: 'failed',
  timeout: 'ran out of time',
  stopped: 'stopped',
}

function SandboxRow({ s }: { s: Sandbox }) {
  const status = s.status === 'failed' && s.exitCode > 0 ? `exited ${s.exitCode}` : sandboxStatus[s.status]
  const took = s.finished ? `${Math.max(1, s.finished - s.started)}s` : ''
  return (
    <li className="text-xs">
      <details className="group">
        <summary className="flex cursor-pointer list-none items-start justify-between gap-3 px-3 py-2 hover:bg-foreground/[0.03]">
          <div className="min-w-0">
            <p className="truncate font-mono">{s.command}</p>
            <p className="text-muted-foreground truncate">
              {s.image} · {s.files} {s.files === 1 ? 'file' : 'files'} in · {s.internet ? 'internet on' : 'no network'}
            </p>
          </div>
          <div className="shrink-0 text-right">
            <p className={s.status === 'ok' ? '' : s.status === 'running' ? 'text-warning' : 'text-danger'}>{status}</p>
            <p className="text-muted-foreground">
              {relTime(s.started)}
              {took && ` · ${took}`}
            </p>
          </div>
        </summary>
        <pre className="max-h-48 overflow-auto border-t border-foreground/6 bg-inset px-3 py-2 font-mono text-[11px] whitespace-pre-wrap">
          {s.output || 'No output.'}
        </pre>
      </details>
    </li>
  )
}
