import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { queryKeys } from '@/constants/query-keys'
import { fmtCount, relTime } from '@/lib/format'
import { cn } from '@/lib/utils'
import { aiService, type SpanView, type TraceSummary } from '@/services/api.service'

// fmtCost shows small amounts with the precision they need, like $0.0042.
const fmtCost = (v: number) => '$' + (v >= 1 ? v.toFixed(2) : v.toPrecision(2))

const fmtMs = (ms: number) => (ms < 1000 ? `${ms}ms` : ms < 60000 ? `${(ms / 1000).toFixed(1)}s` : `${Math.round(ms / 60000)}m`)

// AgentRuns lists an agent's runs from its traces; each opens to its steps, laid out in time.
export function AgentRuns({ app }: { app: string }) {
  const { data } = useQuery({ queryKey: queryKeys.traces(app), queryFn: () => aiService.traces(app), refetchInterval: 10000 })
  const [open, setOpen] = useState<TraceSummary | null>(null)
  const traces = data?.traces ?? []

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <h3 className="text-xs font-medium">Runs</h3>
      {traces.length === 0 ? (
        <p className="text-muted-foreground rounded-lg border border-dashed border-foreground/10 p-4 text-xs">
          No runs yet. Every model call through Skiff shows here, and an agent using OpenTelemetry sends its own steps
          with no setup: Skiff sets the exporter's address and token for it.
        </p>
      ) : (
        <ul className="divide-y divide-foreground/6 overflow-hidden rounded-lg border border-foreground/8">
          {traces.map((t) => (
            <li key={t.id}>
              <button
                type="button"
                onClick={() => setOpen(t)}
                className="flex w-full cursor-pointer items-start justify-between gap-3 px-3 py-2 text-left text-xs hover:bg-foreground/[0.03]"
              >
                <div className="min-w-0">
                  <p className="truncate font-medium">{t.name || 'Unnamed run'}</p>
                  <p className="text-muted-foreground truncate">
                    {t.spans} {t.spans === 1 ? 'step' : 'steps'}
                    {t.modelCalls > 0 && ` · ${t.modelCalls} model ${t.modelCalls === 1 ? 'call' : 'calls'}`}
                    {t.toolCalls > 0 && ` · ${t.toolCalls} tool ${t.toolCalls === 1 ? 'call' : 'calls'}`}
                    {t.inputTokens + t.outputTokens > 0 && ` · ${fmtCount(t.inputTokens + t.outputTokens)} tokens`}
                    {t.cost !== undefined && ` · ${t.costPartial ? 'at least ' : ''}${fmtCost(t.cost)}`}
                  </p>
                </div>
                <div className="shrink-0 text-right">
                  <p className={t.errors > 0 ? 'text-danger' : ''}>
                    {t.errors > 0 ? `${t.errors} ${t.errors === 1 ? 'error' : 'errors'}` : fmtMs(t.durationMs)}
                  </p>
                  <p className="text-muted-foreground">{relTime(Math.floor(t.start / 1000))}</p>
                </div>
              </button>
            </li>
          ))}
        </ul>
      )}
      <RunDialog app={app} run={open} onClose={() => setOpen(null)} />
    </div>
  )
}

function RunDialog({ app, run, onClose }: { app: string; run: TraceSummary | null; onClose: () => void }) {
  const { data } = useQuery({
    queryKey: queryKeys.trace(app, run?.id ?? ''),
    queryFn: () => aiService.trace(app, run!.id),
    enabled: !!run,
  })
  const [picked, setPicked] = useState<string | null>(null)
  const spans = useMemo(() => data?.spans ?? [], [data])
  const depth = useMemo(() => {
    const d: Record<string, number> = {}
    for (const s of spans) d[s.id] = s.parent && d[s.parent] !== undefined ? d[s.parent] + 1 : 0
    return d
  }, [spans])
  const start = spans.length ? Math.min(...spans.map((s) => s.startNs)) : 0
  const end = spans.length ? Math.max(...spans.map((s) => s.endNs)) : 1
  const total = Math.max(end - start, 1)
  const selected = spans.find((s) => s.id === picked)

  return (
    <Dialog
      open={!!run}
      onOpenChange={(o) => {
        if (!o) {
          setPicked(null)
          onClose()
        }
      }}
    >
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{run?.name || 'Run'}</DialogTitle>
          <DialogDescription>
            {run && `${fmtMs(run.durationMs)} · ${run.spans} steps`}
            {run && run.inputTokens + run.outputTokens > 0 && ` · ${fmtCount(run.inputTokens)} tokens in, ${fmtCount(run.outputTokens)} out`}
            {run?.cost !== undefined && ` · ${run.costPartial ? 'at least ' : ''}${fmtCost(run.cost)}`}
          </DialogDescription>
        </DialogHeader>
        <ul className="max-h-[50vh] overflow-auto rounded-lg border border-foreground/8 text-xs">
          {spans.map((s) => (
            <SpanRow
              key={s.id}
              s={s}
              depth={depth[s.id] ?? 0}
              left={((s.startNs - start) / total) * 100}
              width={Math.max(((s.endNs - s.startNs) / total) * 100, 0.5)}
              active={s.id === picked}
              onPick={() => setPicked(s.id === picked ? null : s.id)}
            />
          ))}
        </ul>
        {selected && (
          <div className="min-w-0 rounded-lg border border-foreground/8 p-3 text-xs">
            <p className="font-medium">{selected.name}</p>
            {selected.message && <p className="mt-1 text-danger">{selected.message}</p>}
            <dl className="mt-2 grid max-h-48 grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-4 gap-y-1 overflow-auto font-mono text-[11px]">
              {Object.entries(selected.attrs ?? {}).map(([k, v]) => (
                <div key={k} className="contents">
                  <dt className="text-muted-foreground truncate">{k}</dt>
                  <dd className="break-all">{typeof v === 'string' ? v : JSON.stringify(v)}</dd>
                </div>
              ))}
            </dl>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function SpanRow({
  s,
  depth,
  left,
  width,
  active,
  onPick,
}: {
  s: SpanView
  depth: number
  left: number
  width: number
  active: boolean
  onPick: () => void
}) {
  const ms = Math.round((s.endNs - s.startNs) / 1e6)
  return (
    <li className={cn('border-b border-foreground/5 last:border-0', active && 'bg-foreground/[0.04]')}>
      <button
        type="button"
        onClick={onPick}
        className="grid w-full cursor-pointer grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2.75rem] items-center gap-3 px-3 py-1.5 text-left sm:grid-cols-[minmax(0,3fr)_minmax(0,2fr)_2.75rem]"
      >
        <span className="flex min-w-0 items-center gap-1.5" style={{ paddingLeft: `${Math.min(depth, 8) * 12}px` }}>
          <span className={cn('truncate', s.status === 2 && 'text-danger')}>{s.name}</span>
          {s.source === 'gateway' && (
            <span className="text-muted-foreground shrink-0 rounded bg-foreground/6 px-1 font-mono text-[9px] uppercase max-sm:hidden">skiff</span>
          )}
          {(s.inputTokens ?? 0) + (s.outputTokens ?? 0) > 0 && (
            <span className="text-muted-foreground shrink-0 font-mono text-[10px] max-sm:hidden">
              {fmtCount((s.inputTokens ?? 0) + (s.outputTokens ?? 0))} tok
            </span>
          )}
          {s.cost !== undefined && (
            <span className="text-muted-foreground shrink-0 font-mono text-[10px] max-sm:hidden">{fmtCost(s.cost)}</span>
          )}
        </span>
        <span className="relative h-4">
          <span
            className={cn('absolute top-1 h-2 rounded-sm', s.status === 2 ? 'bg-danger/70' : s.model || s.source === 'gateway' ? 'bg-primary/70' : 'bg-foreground/35')}
            style={{ left: `${left}%`, width: `${width}%` }}
          />
        </span>
        <span className="text-muted-foreground text-right font-mono text-[10px]">{fmtMs(ms)}</span>
      </button>
    </li>
  )
}
