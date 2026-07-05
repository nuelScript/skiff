import {
  GitBranch,
  ExternalLink,
  ScrollText,
  History,
  KeyRound,
  Square,
  MoreHorizontal,
} from 'lucide-react'
import { Link } from 'react-router'
import type { App } from '@/services/api.service'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

type StatusStyle = { dot: string; pill: string; edge: string; pulse: boolean }

function statusStyle(state: string): StatusStyle {
  switch (state) {
    case 'running':
      return {
        dot: 'bg-emerald-400',
        pill: 'border-emerald-400/25 bg-emerald-400/10 text-emerald-300',
        edge: 'bg-emerald-400/50',
        pulse: false,
      }
    case 'exited':
    case 'missing':
      return {
        dot: 'bg-rose-400',
        pill: 'border-rose-400/25 bg-rose-400/10 text-rose-300',
        edge: 'bg-rose-400/50',
        pulse: false,
      }
    case 'building':
    case 'starting':
    case 'created':
      return {
        dot: 'bg-amber-400 text-amber-400',
        pill: 'border-amber-400/25 bg-amber-400/10 text-amber-300',
        edge: 'bg-amber-400/50',
        pulse: true,
      }
    default:
      return {
        dot: 'bg-white/40',
        pill: 'border-white/15 bg-white/5 text-muted-foreground',
        edge: 'bg-white/15',
        pulse: false,
      }
  }
}

export function AppCard({
  app,
  index = 0,
  onLogs,
  onHistory,
  onEnv,
  onStop,
}: {
  app: App
  index?: number
  onLogs: (name: string) => void
  onHistory: (name: string) => void
  onEnv: (name: string) => void
  onStop: (name: string) => void
}) {
  const s = statusStyle(app.state)
  return (
    <article
      style={{ animationDelay: `${Math.min(index, 10) * 45}ms` }}
      className="animate-rise group relative flex flex-col gap-4 overflow-hidden rounded-xl border border-white/8 bg-linear-to-b from-white/2.5 to-transparent p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-white/20 hover:shadow-[0_10px_40px_-15px_rgba(0,0,0,0.9)]"
    >
      <span className={'absolute top-0 left-0 h-full w-[2px] ' + s.edge} />

      <header className="flex items-start justify-between gap-3">
        <Link
          to={'/projects/' + app.name}
          className="hover:text-foreground min-w-0 truncate text-[15px] font-semibold tracking-tight hover:underline"
        >
          {app.name}
        </Link>
        <span
          className={
            'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2 py-0.5 font-mono text-[10px] font-medium tracking-wider uppercase ' +
            s.pill
          }
        >
          <span className={'h-1.5 w-1.5 rounded-full ' + s.dot + (s.pulse ? ' pulse-dot' : '')} />
          {app.state}
        </span>
      </header>

      {app.repo && (
        <div className="text-muted-foreground flex min-w-0 items-center gap-2 text-xs">
          <GitBranch className="h-3.5 w-3.5 shrink-0" />
          <span className="truncate font-mono">{app.repo}</span>
          {app.branch && <span className="shrink-0 font-mono text-white/35">{app.branch}</span>}
          {app.auto && (
            <span className="shrink-0 rounded border border-emerald-400/30 px-1.5 py-px font-mono text-[9px] font-medium tracking-wider text-emerald-300/90 uppercase">
              auto
            </span>
          )}
        </div>
      )}

      <a
        href={app.url}
        target="_blank"
        rel="noreferrer"
        className="group/url text-muted-foreground hover:text-foreground flex min-w-0 items-center gap-1.5 font-mono text-xs transition-colors"
      >
        <span className="truncate">{app.url.replace(/^https?:\/\//, '')}</span>
        <ExternalLink className="h-3 w-3 shrink-0 opacity-0 transition-opacity group-hover/url:opacity-100" />
      </a>

      <div className="mt-auto flex items-center gap-1 pt-1">
        <Button size="sm" variant="ghost" onClick={() => onLogs(app.name)}>
          <ScrollText />
          Logs
        </Button>
        <Button size="sm" variant="ghost" onClick={() => onHistory(app.name)}>
          <History />
          History
        </Button>
        <div className="flex-1" />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="icon-sm" variant="ghost" className="text-muted-foreground" title="More">
              <MoreHorizontal />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-44">
            <DropdownMenuItem onClick={() => onEnv(app.name)} className="gap-2">
              <KeyRound className="h-3.5 w-3.5" />
              Environment
            </DropdownMenuItem>
            <DropdownMenuItem asChild className="gap-2">
              <a href={app.url} target="_blank" rel="noreferrer">
                <ExternalLink className="h-3.5 w-3.5" />
                Visit site
              </a>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={() => onStop(app.name)}
              className="gap-2 text-rose-400 focus:text-rose-400"
            >
              <Square className="h-3.5 w-3.5" />
              Stop
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </article>
  )
}
