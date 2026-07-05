import { useState } from 'react'
import { Activity } from 'lucide-react'
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
} from 'recharts'
import { useApps } from '@/hooks/use-apps'
import { useAnalytics } from '@/hooks/use-analytics'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Analytics } from '@/services/api.service'

const RANGES = [
  { v: 60, label: 'Last hour' },
  { v: 360, label: 'Last 6 hours' },
  { v: 720, label: 'Last 12 hours' },
  { v: 1440, label: 'Last 24 hours' },
]

const STATUS = [
  { key: 's2' as const, label: '2XX', color: '#34d399' },
  { key: 's3' as const, label: '3XX', color: '#38bdf8' },
  { key: 's4' as const, label: '4XX', color: '#fbbf24' },
  { key: 's5' as const, label: '5XX', color: '#fb7185' },
]

const axisTick = { fill: '#71717a', fontSize: 10, fontFamily: 'ui-monospace, monospace' }
const gridStroke = 'rgba(255,255,255,0.05)'

const fmtClock = (t: number) =>
  new Date(t * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

const timeRange = (t: number, secs: number) => `${fmtClock(t)} – ${fmtClock(t + secs)}`

function fmtBytes(b: number): string {
  if (b >= 1 << 30) return (b / (1 << 30)).toFixed(1) + 'GB'
  if (b >= 1 << 20) return (b / (1 << 20)).toFixed(1) + 'MB'
  if (b >= 1 << 10) return (b / (1 << 10)).toFixed(0) + 'kB'
  return b + 'B'
}

export default function AnalyticsPage() {
  const { apps } = useApps()
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
              {(a?.appOptions ?? apps.map((x) => x.name)).map((n) => (
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

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="rounded-xl border border-white/8 bg-linear-to-b from-white/2 to-transparent p-5">
      <h2 className="mb-3 text-sm font-semibold">{title}</h2>
      {children}
    </section>
  )
}

function TipCard({
  title,
  rows,
}: {
  title: string
  rows: { label: string; color: string; value: string }[]
}) {
  return (
    <div className="rounded-lg border border-white/10 bg-black/90 px-3 py-2 shadow-xl backdrop-blur-sm">
      <p className="text-muted-foreground mb-1.5 font-mono text-[10px]">{title}</p>
      <div className="space-y-1">
        {rows.map((r) => (
          <div key={r.label} className="flex items-center justify-between gap-6 text-xs">
            <span className="flex items-center gap-1.5">
              <span className="h-2 w-2 rounded-full" style={{ background: r.color }} />
              {r.label}
            </span>
            <span className="font-mono tabular-nums">{r.value}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

type Row = Record<string, number>

function ChartTip(props: {
  active?: boolean
  payload?: { payload: Row }[]
  build?: (row: Row) => { title: string; rows: { label: string; color: string; value: string }[] }
}) {
  const { active, payload, build } = props
  if (!active || !payload?.length || !build) return null
  const { title, rows } = build(payload[0].payload)
  return <TipCard title={title} rows={rows} />
}

function EdgeRequests({ a }: { a: Analytics }) {
  return (
    <Panel title="Edge Requests">
      <div className="mb-3 flex flex-wrap items-center gap-4">
        <span className="text-2xl font-semibold tracking-tight tabular-nums">
          {a.total.toLocaleString()}
        </span>
        <div className="flex flex-wrap gap-x-3 gap-y-1">
          {STATUS.map((s) => (
            <span key={s.key} className="text-muted-foreground flex items-center gap-1.5 text-[11px]">
              <span className="h-1.5 w-1.5 rounded-full" style={{ background: s.color }} />
              {s.label} {a.status[s.key].toLocaleString()}
            </span>
          ))}
        </div>
      </div>
      <ResponsiveContainer width="100%" height={150}>
        <BarChart data={a.series} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
          <CartesianGrid vertical={false} stroke={gridStroke} />
          <XAxis
            dataKey="t"
            tickFormatter={fmtClock}
            interval="preserveStartEnd"
            minTickGap={80}
            tick={axisTick}
            tickLine={false}
            axisLine={false}
          />
          <YAxis allowDecimals={false} width={34} tick={axisTick} tickLine={false} axisLine={false} />
          <Tooltip
            cursor={{ fill: 'rgba(255,255,255,0.05)' }}
            content={
              <ChartTip
                build={(row) => ({
                  title: timeRange(row.t, a.bucketSecs),
                  rows: STATUS.map((s) => ({ label: s.label, color: s.color, value: String(row[s.key]) })),
                })}
              />
            }
          />
          {STATUS.map((s, i) => (
            <Bar
              key={s.key}
              dataKey={s.key}
              stackId="s"
              fill={s.color}
              maxBarSize={16}
              radius={i === STATUS.length - 1 ? [2, 2, 0, 0] : 0}
              isAnimationActive={false}
            />
          ))}
        </BarChart>
      </ResponsiveContainer>
    </Panel>
  )
}

function DataTransfer({ a }: { a: Analytics }) {
  return (
    <Panel title="Data Transfer">
      <div className="mb-3 flex gap-6">
        <Metric dot="#fbbf24" label="Incoming" value={fmtBytes(a.bytesIn)} />
        <Metric dot="#38bdf8" label="Outgoing" value={fmtBytes(a.bytesOut)} />
      </div>
      <ResponsiveContainer width="100%" height={150}>
        <AreaChart data={a.series} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id="bo" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#38bdf8" stopOpacity={0.25} />
              <stop offset="100%" stopColor="#38bdf8" stopOpacity={0} />
            </linearGradient>
            <linearGradient id="bi" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#fbbf24" stopOpacity={0.2} />
              <stop offset="100%" stopColor="#fbbf24" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid vertical={false} stroke={gridStroke} />
          <XAxis
            dataKey="t"
            tickFormatter={fmtClock}
            interval="preserveStartEnd"
            minTickGap={80}
            tick={axisTick}
            tickLine={false}
            axisLine={false}
          />
          <YAxis width={44} tickFormatter={fmtBytes} tick={axisTick} tickLine={false} axisLine={false} />
          <Tooltip
            cursor={{ stroke: 'rgba(255,255,255,0.15)' }}
            content={
              <ChartTip
                build={(row) => ({
                  title: timeRange(row.t, a.bucketSecs),
                  rows: [
                    { label: 'Outgoing', color: '#38bdf8', value: fmtBytes(row.bo) },
                    { label: 'Incoming', color: '#fbbf24', value: fmtBytes(row.bi) },
                  ],
                })}
              />
            }
          />
          <Area dataKey="bo" stroke="#38bdf8" strokeWidth={1.5} fill="url(#bo)" isAnimationActive={false} />
          <Area dataKey="bi" stroke="#fbbf24" strokeWidth={1.5} fill="url(#bi)" isAnimationActive={false} />
        </AreaChart>
      </ResponsiveContainer>
    </Panel>
  )
}

function Latency({ a }: { a: Analytics }) {
  return (
    <Panel title="Latency">
      <div className="mb-3 flex items-baseline gap-2">
        <span className="text-2xl font-semibold tracking-tight tabular-nums">{a.avgLatMs}</span>
        <span className="text-muted-foreground text-xs">ms average</span>
      </div>
      <ResponsiveContainer width="100%" height={150}>
        <AreaChart data={a.series} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id="lat" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#a78bfa" stopOpacity={0.25} />
              <stop offset="100%" stopColor="#a78bfa" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid vertical={false} stroke={gridStroke} />
          <XAxis
            dataKey="t"
            tickFormatter={fmtClock}
            interval="preserveStartEnd"
            minTickGap={80}
            tick={axisTick}
            tickLine={false}
            axisLine={false}
          />
          <YAxis width={32} tick={axisTick} tickLine={false} axisLine={false} />
          <Tooltip
            cursor={{ stroke: 'rgba(255,255,255,0.15)' }}
            content={
              <ChartTip
                build={(row) => ({
                  title: timeRange(row.t, a.bucketSecs),
                  rows: [{ label: 'Avg latency', color: '#a78bfa', value: row.lat + 'ms' }],
                })}
              />
            }
          />
          <Area dataKey="lat" stroke="#a78bfa" strokeWidth={1.5} fill="url(#lat)" isAnimationActive={false} />
        </AreaChart>
      </ResponsiveContainer>
    </Panel>
  )
}

function Metric({ dot, label, value }: { dot: string; label: string; value: string }) {
  return (
    <div>
      <p className="text-muted-foreground flex items-center gap-1.5 text-[11px]">
        <span className="h-1.5 w-1.5 rounded-full" style={{ background: dot }} />
        {label}
      </p>
      <p className="mt-0.5 text-lg font-semibold tabular-nums">{value}</p>
    </div>
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
