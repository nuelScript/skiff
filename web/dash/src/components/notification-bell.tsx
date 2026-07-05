import { useState } from 'react'
import { useNavigate } from 'react-router'
import { Bell } from 'lucide-react'
import { useAllDeploys } from '@/hooks/use-all-deploys'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
} from '@/components/ui/dropdown-menu'

const SEEN_KEY = 'skiff-notifs-seen'

function rel(unix: number): string {
  const s = Math.max(0, Math.floor(Date.now() / 1000 - unix))
  if (s < 60) return s + 's'
  if (s < 3600) return Math.floor(s / 60) + 'm'
  if (s < 86400) return Math.floor(s / 3600) + 'h'
  return Math.floor(s / 86400) + 'd'
}

const dot = (status: string): string =>
  status === 'live'
    ? 'bg-emerald-400'
    : status === 'failed'
      ? 'bg-rose-400'
      : 'bg-amber-400 pulse-dot'

export default function NotificationBell() {
  const { data: deploys = [] } = useAllDeploys()
  const navigate = useNavigate()
  const [seen, setSeen] = useState<number>(() => Number(localStorage.getItem(SEEN_KEY) || 0))

  const recent = deploys.slice(0, 8)
  const unread = recent.filter((d) => d.started > seen).length

  const markSeen = () => {
    const latest = recent[0]?.started ?? Math.floor(Date.now() / 1000)
    localStorage.setItem(SEEN_KEY, String(latest))
    setSeen(latest)
  }

  return (
    <DropdownMenu
      onOpenChange={(o) => {
        if (o) markSeen()
      }}
    >
      <DropdownMenuTrigger asChild>
        <button
          aria-label="Activity"
          className="text-muted-foreground hover:text-foreground relative grid h-8 w-8 place-items-center rounded-md transition-colors hover:bg-white/5"
        >
          <Bell className="h-4 w-4" />
          {unread > 0 && (
            <span className="absolute top-1.5 right-1.5 h-2 w-2 rounded-full bg-emerald-400 ring-2 ring-background" />
          )}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80 p-0">
        <div className="border-b border-white/8 px-3 py-2 text-xs font-medium">Activity</div>
        {recent.length === 0 ? (
          <p className="text-muted-foreground p-6 text-center text-xs">No deployments yet.</p>
        ) : (
          <div className="max-h-96 overflow-auto py-1">
            {recent.map((d) => (
              <button
                key={d.app + d.id}
                onClick={() => navigate('/projects/' + d.app)}
                className="flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors hover:bg-white/5"
              >
                <span className={'h-1.5 w-1.5 shrink-0 rounded-full ' + dot(d.status)} />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs font-medium">
                    <span className="font-mono">{d.app}</span>{' '}
                    <span className="text-muted-foreground font-normal">
                      {d.status === 'live' ? 'deployed' : d.status}
                    </span>
                  </p>
                  <p className="text-muted-foreground truncate text-[11px]">
                    {d.message || d.commit || d.trigger}
                  </p>
                </div>
                <span className="text-muted-foreground shrink-0 font-mono text-[10px] tabular-nums">
                  {rel(d.started)}
                </span>
              </button>
            ))}
          </div>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
