import { Activity } from 'lucide-react'
import { useAnalytics } from '@/hooks/use-analytics'
import type { Analytics, AnalyticsPoint } from '@/services/api.service'

const fmtTime = (unix: number) =>
  new Date(unix * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

const STATUS = [
  { key: 's2', label: '2xx', color: 'bg-emerald-400', text: 'text-emerald-300' },
  { key: 's3', label: '3xx', color: 'bg-sky-400', text: 'text-sky-300' },
  { key: 's4', label: '4xx', color: 'bg-amber-400', text: 'text-amber-300' },
  { key: 's5', label: '5xx', color: 'bg-rose-400', text: 'text-rose-300' },
] as const

export default function AnalyticsPage() {
  const { data: a } = useAnalytics()

  if (!a) {
    return <div className="text-muted-foreground px-8 py-8 text-sm">Loading analytics…</div>
  }

  const avgLat = a.apps.length
    ? Math.round(a.apps.reduce((s, x) => s + x.avgLatMs * x.req, 0) / Math.max(1, a.total))
    : 0

  return (
    <div className="px-8 py-8">
      <header className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight">Analytics</h1>
        <p className="text-muted-foreground mt-1 text-sm">
          Requests your edge router served across your apps — last {a.windowMins} minutes.
        </p>
      </header>

      {a.total === 0 ? (
        <div className="text-muted-foreground flex flex-col items-center gap-2 rounded-xl border border-white/8 py-20 text-sm">
          <Activity className="h-6 w-6 opacity-40" />
          <span>No traffic recorded in the last hour.</span>
        </div>
      ) : (
        <div className="space-y-4">
          {/* stat cards */}
          <div className="grid gap-4 sm:grid-cols-3">
            <Stat label="Requests" value={a.total.toLocaleString()} sub="last hour" />
            <Stat label="Avg latency" value={avgLat + ' ms'} sub="per request" />
            <Stat
              label="Success"
              value={a.total ? Math.round((a.status.s2 / a.total) * 100) + '%' : '—'}
              sub={`${a.status.s4 + a.status.s5} errors`}
            />
          </div>

          {/* traffic over time */}
          <section className="rounded-xl border border-white/8 p-5">
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-muted-foreground font-mono text-[11px] tracking-wider uppercase">
                Requests / min
              </h2>
              <span className="text-muted-foreground font-mono text-[11px] tabular-nums">
                peak {Math.max(...a.series.map((p) => p.req))}
              </span>
            </div>
            <TrafficChart series={a.series} />
            <div className="text-muted-foreground mt-2 flex justify-between font-mono text-[10px]">
              <span>{fmtTime(a.series[0].t)}</span>
              <span>{fmtTime(a.series[a.series.length - 1].t)}</span>
            </div>
          </section>

          {/* status + top apps */}
          <div className="grid gap-4 lg:grid-cols-2">
            <StatusBreakdown a={a} />
            <TopApps a={a} />
          </div>
        </div>
      )}
    </div>
  )
}

function Stat({ label, value, sub }: { label: string; value: string; sub: string }) {
  return (
    <div className="rounded-xl border border-white/8 bg-linear-to-b from-white/2.5 to-transparent p-5">
      <p className="text-muted-foreground font-mono text-[11px] tracking-wider uppercase">{label}</p>
      <p className="mt-1.5 text-2xl font-semibold tracking-tight tabular-nums">{value}</p>
      <p className="text-muted-foreground mt-1 text-xs">{sub}</p>
    </div>
  )
}

function TrafficChart({ series }: { series: AnalyticsPoint[] }) {
  const max = Math.max(1, ...series.map((p) => p.req))
  const W = 600
  const H = 100
  const bw = W / series.length
  return (
    <div className="h-32">
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="h-full w-full">
        {series.map((p, i) => {
          const h = p.req > 0 ? Math.max(1.5, (p.req / max) * H) : 0
          return (
            <rect
              key={p.t}
              x={i * bw}
              y={H - h}
              width={Math.max(0.5, bw - 0.6)}
              height={h}
              className={p.req > 0 ? 'fill-emerald-400/70' : 'fill-white/5'}
            >
              <title>{`${fmtTime(p.t)} — ${p.req} req`}</title>
            </rect>
          )
        })}
      </svg>
    </div>
  )
}

function StatusBreakdown({ a }: { a: Analytics }) {
  return (
    <section className="rounded-xl border border-white/8 p-5">
      <h2 className="text-muted-foreground mb-3 font-mono text-[11px] tracking-wider uppercase">
        Status codes
      </h2>
      <div className="mb-4 flex h-2.5 overflow-hidden rounded-full bg-white/5">
        {STATUS.map((s) => {
          const v = a.status[s.key]
          const pct = a.total ? (v / a.total) * 100 : 0
          return pct > 0 ? <div key={s.key} className={s.color} style={{ width: pct + '%' }} /> : null
        })}
      </div>
      <div className="grid grid-cols-2 gap-x-6 gap-y-2">
        {STATUS.map((s) => (
          <div key={s.key} className="flex items-center justify-between text-sm">
            <span className="flex items-center gap-2">
              <span className={'h-2 w-2 rounded-full ' + s.color} />
              <span className={s.text}>{s.label}</span>
            </span>
            <span className="text-muted-foreground font-mono text-xs tabular-nums">
              {a.status[s.key].toLocaleString()}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}

function TopApps({ a }: { a: Analytics }) {
  const max = Math.max(1, ...a.apps.map((x) => x.req))
  return (
    <section className="rounded-xl border border-white/8 p-5">
      <h2 className="text-muted-foreground mb-3 font-mono text-[11px] tracking-wider uppercase">
        Top apps
      </h2>
      {a.apps.length === 0 ? (
        <p className="text-muted-foreground text-sm">No app traffic yet.</p>
      ) : (
        <div className="space-y-2.5">
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
    </section>
  )
}
