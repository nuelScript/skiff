import { GitBranch } from 'lucide-react'
import type { SystemInfo } from '@/services/api.service'
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { LogoMark } from '@/components/logo'

const statusDot = (s: string): string =>
  s === 'live'
    ? 'bg-emerald-500'
    : s === 'failed'
      ? 'bg-red-500'
      : 'bg-amber-500'

function rel(unix: number): string {
  const s = Math.max(0, Math.floor(Date.now() / 1000 - unix))
  if (s < 60) return s + 's ago'
  if (s < 3600) return Math.floor(s / 60) + 'm ago'
  if (s < 86400) return Math.floor(s / 3600) + 'h ago'
  return Math.floor(s / 86400) + 'd ago'
}

export default function ControlPlaneCard({
  info,
  onHistory,
  onLogs,
}: {
  info: SystemInfo
  onHistory: (app: string) => void
  onLogs: (app: string, id: string) => void
}) {
  const latest = info.deploys[0]
  const building = latest?.status === 'building'
  return (
    <Card
      size="sm"
      className="border-foreground/20 bg-foreground/[0.02] ring-foreground/5 ring-1 ring-inset"
    >
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LogoMark className="h-4 w-4" />
          Control plane
        </CardTitle>
        <CardAction>
          <span className="text-muted-foreground flex items-center gap-1.5 font-mono text-[11px] tracking-wide uppercase">
            <span
              className={
                'h-1.5 w-1.5 rounded-full ' +
                (building ? 'bg-amber-500' : 'bg-emerald-500')
              }
            />
            {building ? 'deploying' : 'running'}
          </span>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {info.repo && (
          <div className="text-muted-foreground flex items-center gap-2 text-xs">
            <GitBranch className="h-3.5 w-3.5 shrink-0" />
            <span className="truncate font-mono">{info.repo}</span>
            <span className="rounded border border-emerald-500/40 px-1.5 py-0.5 font-mono text-[10px] tracking-wide text-emerald-500 uppercase">
              self-deploys
            </span>
          </div>
        )}

        {latest ? (
          <div className="flex items-center gap-2 font-mono text-xs">
            <span className={'h-1.5 w-1.5 shrink-0 rounded-full ' + statusDot(latest.status)} />
            <span className="text-foreground">{latest.commit || latest.status}</span>
            <span className="text-muted-foreground truncate">
              {latest.trigger} · {rel(latest.started)}
            </span>
          </div>
        ) : (
          <p className="text-muted-foreground text-xs">
            Rebuilds and hot-swaps itself on every push — zero downtime.
          </p>
        )}

        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={() => onHistory('panel')}>
            History
          </Button>
          {latest && (
            <Button size="sm" variant="outline" onClick={() => onLogs('panel', latest.id)}>
              Logs
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
