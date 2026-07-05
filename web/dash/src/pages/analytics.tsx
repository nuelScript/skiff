import { useMemo, useState } from 'react'
import { Activity } from 'lucide-react'
import { useAnalytics } from '@/hooks/use-analytics'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Analytics, AnalyticsSeries } from '@/services/api.service'

const RANGES = [
  { v: 60, label: 'Last hour' },
  { v: 360, label: 'Last 6 hours' },
  { v: 720, label: 'Last 12 hours' },
  { v: 1440, label: 'Last 24 hours' },
]

const STATUS_LAYERS = [
  { key: 's2' as const, label: '2XX', fill: 'fill-emerald-400/70', dot: 'bg-emerald-400' },
  { key: 's3' as const, label: '3XX', fill: 'fill-sky-400/70', dot: 'bg-sky-400' },
  { key: 's4' as const, label: '4XX', fill: 'fill-amber-400/70', dot: 'bg-amber-400' },
  { key: 's5' as const, label: '5XX', fill: 'fill-rose-400/80', dot: 'bg-rose-400' },
]

const fmtTime = (unix: number) =>
  new Date(unix * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

function fmtBytes(b: number): string {
  if (b >= 1 << 30) return (b / (1 << 30)).toFixed(1) + 'GB'
  if (b >= 1 << 20) return (b / (1 << 20)).toFixed(1) + 'MB'
  if (b >= 1 << 10) return (b / (1 << 10)).toFixed(0) + 'kB'
  return b + 'B'
}

export default function AnalyticsPage() {
  const [range, setRange] = useState(60)
  const [app, setApp] = useState('')
  const { data: a } = useAnalytics(range, app)

  const rangeLabel = RANGES.find((r) => r.v === range)?.label ?? 'Last hour'

  return (
    <div className="px-8 py-8">
      <header className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Analytics</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Traffic your edge router served — {rangeLabel.toLowerCase()}.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Select value={app || 'all'} onValueChange={(v) => setApp(v === 'all' ? '' : v)}>
            <SelectTrigger size="sm" className="w-40 bg-white/2">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All apps</SelectItem>
              {(a?.appOptions ?? []).map((n) => (
                <SelectItem key={n} value={n} className="font-mono">
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={String(range)} onValueChange={(v) => setRange(Number(v))}>
            <SelectTrigger size="sm" className="w-40 bg-white/2">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {RANGES.map((r) => (
                <SelectItem key={r.v} value={String(r.v)}>
                  {r.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </header>

      {!a ? (
        <p className="text-muted-foreground text-sm">Loading analytics…</p>
      ) : a.total === 0 ? (
        <div className="text-muted-foreground flex flex-col items-center gap-2 rounded-xl border border-white/8 py-20 text-sm">
          <Activity className="h-6 w-6 opacity-40" />
          <span>No traffic in this range.</span>
        </div>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          <EdgeRequests a={a} />
          <DataTransfer a={a} />
          <Latency a={a} />
          <TopApps a={a} />
        </div>
      )}
    </div>
  )
}

function Panel({
  title,
  head,
  children,
}: {
  title: string
  head?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section className="rounded-xl border border-white/8 bg-linear-to-b from-white/2 to-transparent p-5">
      <div className="mb-3 flex items-start justify-between gap-4">
        <h2 className="text-sm font-semibold">{title}</h2>
      </div>
      {head}
      {children}
    </section>
  )
}

const axis = (s: AnalyticsSeries[]) => (
  <div className="text-muted-foreground mt-2 flex justify-between font-mono text-[10px]">
    <span>{fmtTime(s[0].t)}</span>
    <span>{fmtTime(s[s.length - 1].t)}</span>
  </div>
)

// Chart renders stacked areas or overlaid lines over a time series. Non-uniform
// scaling is fine for filled areas; strokes use non-scaling width.
function Chart({
  layers,
  max,
  stacked,
  height = 132,
}: {
  layers: { values: number[]; className: string }[]
  max: number
  stacked?: boolean
  height?: number
}) {
  const n = layers[0]?.values.length ?? 0
  const W = 600
  const H = 100
  const xi = (i: number) => (n <= 1 ? 0 : (i / (n - 1)) * W)
  const yv = (v: number) => H - (max > 0 ? Math.min(1, v / max) * H : 0)
  if (n === 0) return <div style={{ height }} />

  const paths = stacked
    ? layers.map((_, li) => {
        const lower: number[] = []
        const upper: number[] = []
        for (let i = 0; i < n; i++) {
          let lo = 0
          for (let k = 0; k < li; k++) lo += layers[k].values[i]
          lower.push(lo)
          upper.push(lo + layers[li].values[i])
        }
        let d = 'M' + upper.map((v, i) => `${xi(i)} ${yv(v)}`).join(' L')
        for (let i = n - 1; i >= 0; i--) d += ` L${xi(i)} ${yv(lower[i])}`
        return { d: d + ' Z', className: layers[li].className, fill: true }
      })
    : layers.map((l) => ({
        d: 'M' + l.values.map((v, i) => `${xi(i)} ${yv(v)}`).join(' L'),
        className: l.className,
        fill: false,
      }))

  return (
    <div style={{ height }}>
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="h-full w-full">
        {paths.map((p, i) =>
          p.fill ? (
            <path key={i} d={p.d} className={p.className} />
          ) : (
            <path
              key={i}
              d={p.d}
              fill="none"
              strokeWidth={1.5}
              vectorEffect="non-scaling-stroke"
              className={p.className}
            />
          ),
        )}
      </svg>
    </div>
  )
}

function EdgeRequests({ a }: { a: Analytics }) {
  const max = useMemo(
    () => Math.max(1, ...a.series.map((s) => s.s2 + s.s3 + s.s4 + s.s5)),
    [a.series],
  )
  const layers = STATUS_LAYERS.map((l) => ({
    values: a.series.map((s) => s[l.key]),
    className: l.fill,
  }))
  return (
    <Panel title="Edge Requests">
      <div className="mb-3 flex items-center gap-4">
        <span className="text-2xl font-semibold tracking-tight tabular-nums">
          {a.total.toLocaleString()}
        </span>
        <div className="flex flex-wrap gap-x-3 gap-y-1">
          {STATUS_LAYERS.map((l) => (
            <span key={l.key} className="text-muted-foreground flex items-center gap-1.5 text-[11px]">
              <span className={'h-1.5 w-1.5 rounded-full ' + l.dot} />
              {l.label} {a.status[l.key].toLocaleString()}
            </span>
          ))}
        </div>
      </div>
      <Chart layers={layers} max={max} stacked />
      {axis(a.series)}
    </Panel>
  )
}

function DataTransfer({ a }: { a: Analytics }) {
  const max = useMemo(() => Math.max(1, ...a.series.map((s) => Math.max(s.bi, s.bo))), [a.series])
  return (
    <Panel title="Data Transfer">
      <div className="mb-3 flex gap-6">
        <div>
          <p className="text-muted-foreground flex items-center gap-1.5 text-[11px]">
            <span className="h-1.5 w-1.5 rounded-full bg-amber-400" /> Incoming
          </p>
          <p className="mt-0.5 text-lg font-semibold tabular-nums">{fmtBytes(a.bytesIn)}</p>
        </div>
        <div>
          <p className="text-muted-foreground flex items-center gap-1.5 text-[11px]">
            <span className="h-1.5 w-1.5 rounded-full bg-sky-400" /> Outgoing
          </p>
          <p className="mt-0.5 text-lg font-semibold tabular-nums">{fmtBytes(a.bytesOut)}</p>
        </div>
      </div>
      <Chart
        layers={[
          { values: a.series.map((s) => s.bo), className: 'stroke-sky-400' },
          { values: a.series.map((s) => s.bi), className: 'stroke-amber-400' },
        ]}
        max={max}
      />
      {axis(a.series)}
    </Panel>
  )
}

function Latency({ a }: { a: Analytics }) {
  const max = useMemo(() => Math.max(1, ...a.series.map((s) => s.lat)), [a.series])
  return (
    <Panel title="Latency">
      <div className="mb-3 flex items-baseline gap-2">
        <span className="text-2xl font-semibold tracking-tight tabular-nums">{a.avgLatMs}</span>
        <span className="text-muted-foreground text-xs">ms average</span>
      </div>
      <Chart layers={[{ values: a.series.map((s) => s.lat), className: 'stroke-violet-400' }]} max={max} />
      {axis(a.series)}
    </Panel>
  )
}

function TopApps({ a }: { a: Analytics }) {
  const max = Math.max(1, ...a.apps.map((x) => x.req))
  return (
    <Panel title="Top apps">
      {a.apps.length === 0 ? (
        <p className="text-muted-foreground text-sm">No app traffic yet.</p>
      ) : (
        <div className="space-y-2.5 pt-1">
          {a.apps.map((app) => (
            <div key={app.name} className="grid grid-cols-[1fr_auto] items-center gap-3">
              <div className="min-w-0">
                <div className="mb-1 flex items-center justify-between gap-2">
                  <span className="truncate font-mono text-sm">{app.name}</span>
                  <span className="text-muted-foreground shrink-0 font-mono text-[11px] tabular-nums">
                    {app.avgLatMs}ms
                  </span>
                </div>
                <div className="h-1.5 overflow-hidden rounded-full bg-white/5">
                  <div
                    className="h-full rounded-full bg-emerald-400/60"
                    style={{ width: (app.req / max) * 100 + '%' }}
                  />
                </div>
              </div>
              <span className="font-mono text-sm tabular-nums">{app.req.toLocaleString()}</span>
            </div>
          ))}
        </div>
      )}
    </Panel>
  )
}
