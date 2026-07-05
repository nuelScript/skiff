import { useMemo, useState } from 'react'
import { Link } from 'react-router'
import { Search, GitCommitHorizontal } from 'lucide-react'
import { useAllDeploys } from '@/hooks/use-all-deploys'
import { useConsole } from '@/hooks/use-console'
import { Drawer } from '@/components/drawer'
import type { Deploy } from '@/services/api.service'

function rel(unix: number): string {
  const s = Math.max(0, Math.floor(Date.now() / 1000 - unix))
  if (s < 60) return s + 's ago'
  if (s < 3600) return Math.floor(s / 60) + 'm ago'
  if (s < 86400) return Math.floor(s / 3600) + 'h ago'
  return Math.floor(s / 86400) + 'd ago'
}

const deployDot = (status: string): string =>
  status === 'live'
    ? 'bg-emerald-400'
    : status === 'failed'
      ? 'bg-rose-400'
      : 'bg-amber-400 pulse-dot'

const statusLabel = (status: string): string =>
  status === 'live' ? 'Ready' : status.charAt(0).toUpperCase() + status.slice(1)

type Filter = 'all' | 'live' | 'building' | 'failed'

const FILTERS: { key: Filter; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'live', label: 'Ready' },
  { key: 'building', label: 'Building' },
  { key: 'failed', label: 'Failed' },
]

const matchesFilter = (d: Deploy, f: Filter): boolean =>
  f === 'all'
    ? true
    : f === 'building'
      ? d.status !== 'live' && d.status !== 'failed'
      : d.status === f

export default function DeploymentsPage() {
  const { data: deploys = [], isLoading } = useAllDeploys()
  const term = useConsole(() => {})
  const [q, setQ] = useState('')
  const [filter, setFilter] = useState<Filter>('all')

  const rows = useMemo(() => {
    const needle = q.trim().toLowerCase()
    return deploys.filter((d) => {
      if (!matchesFilter(d, filter)) return false
      if (!needle) return true
      return (
        d.app.toLowerCase().includes(needle) ||
        d.message.toLowerCase().includes(needle) ||
        d.commit.toLowerCase().includes(needle) ||
        d.trigger.toLowerCase().includes(needle)
      )
    })
  }, [deploys, q, filter])

  return (
    <div className="px-8 py-8">
      <header className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight">Deployments</h1>
        <p className="text-muted-foreground mt-1 text-sm">
          Every build across your projects, newest first.
        </p>
      </header>

      {/* toolbar */}
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative sm:max-w-xs sm:flex-1">
          <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2" />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search project, commit, message…"
            className="h-9 w-full rounded-[6px] border border-white/12 bg-white/2 pr-3 pl-8.5 text-sm outline-none placeholder:text-white/30 focus-visible:border-white/25"
          />
        </div>
        <div className="flex shrink-0 items-center gap-1 rounded-[6px] border border-white/10 bg-white/2 p-0.5">
          {FILTERS.map((f) => (
            <button
              key={f.key}
              onClick={() => setFilter(f.key)}
              className={
                'rounded-[4px] px-2.5 py-1 text-xs font-medium transition-colors ' +
                (filter === f.key
                  ? 'text-foreground bg-white/10'
                  : 'text-muted-foreground hover:text-foreground')
              }
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>

      {/* feed */}
      <div className="overflow-hidden rounded-xl border border-white/8">
        {isLoading ? (
          <p className="text-muted-foreground p-8 text-center text-sm">Loading deployments…</p>
        ) : rows.length === 0 ? (
          <p className="text-muted-foreground p-10 text-center text-sm">
            {deploys.length === 0
              ? 'No deployments yet — deploy a project to see its build history here.'
              : 'No deployments match your filters.'}
          </p>
        ) : (
          rows.map((d) => (
            <button
              key={d.app + d.id}
              onClick={() => term.showBuildLog(d.app, d.id)}
              className="flex w-full items-center gap-3.5 border-b border-white/5 px-5 py-3.5 text-left transition-colors last:border-0 hover:bg-white/3"
            >
              <span className={'h-2 w-2 shrink-0 rounded-full ' + deployDot(d.status)} />

              {/* project avatar */}
              <span className="grid h-7 w-7 shrink-0 place-items-center rounded-md border border-white/10 bg-linear-to-br from-white/12 to-white/2 font-mono text-xs font-semibold text-white/80">
                {d.app.charAt(0).toUpperCase()}
              </span>

              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <Link
                    to={'/projects/' + d.app}
                    onClick={(e) => e.stopPropagation()}
                    className="hover:text-foreground truncate text-sm font-medium hover:underline"
                  >
                    {d.app}
                  </Link>
                  <span className="text-muted-foreground shrink-0 text-[11px]">
                    {statusLabel(d.status)}
                  </span>
                </div>
                <p className="text-muted-foreground mt-0.5 truncate text-xs">
                  {d.message || 'No commit message'}
                </p>
              </div>

              {d.commit && (
                <span className="text-muted-foreground hidden shrink-0 items-center gap-1 font-mono text-xs sm:flex">
                  <GitCommitHorizontal className="h-3.5 w-3.5 opacity-60" />
                  {d.commit}
                </span>
              )}
              <span className="text-muted-foreground hidden w-14 shrink-0 truncate font-mono text-[11px] md:block">
                {d.trigger}
              </span>
              <span className="text-muted-foreground w-16 shrink-0 text-right font-mono text-[11px]">
                {rel(d.started)}
              </span>
            </button>
          ))
        )}
      </div>

      {term.stream && <Drawer stream={term.stream} onClose={term.close} />}
    </div>
  )
}
