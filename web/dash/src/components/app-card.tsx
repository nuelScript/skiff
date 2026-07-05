import { GitBranch } from 'lucide-react'
import type { App } from '@/services/api.service'
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'

const statusDot = (state: string): string =>
  state === 'running'
    ? 'bg-emerald-500'
    : state === 'exited' || state === 'missing'
      ? 'bg-red-500'
      : 'bg-muted-foreground'

export default function AppCard({
  app,
  onLogs,
  onHistory,
  onEnv,
  onStop,
}: {
  app: App
  onLogs: (name: string) => void
  onHistory: (name: string) => void
  onEnv: (name: string) => void
  onStop: (name: string) => void
}) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>{app.name}</CardTitle>
        <CardAction>
          <span className="text-muted-foreground flex items-center gap-1.5 font-mono text-[11px] tracking-wide uppercase">
            <span className={'h-1.5 w-1.5 rounded-full ' + statusDot(app.state)} />
            {app.state}
          </span>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {app.repo && (
          <div className="text-muted-foreground flex items-center gap-2 text-xs">
            <GitBranch className="h-3.5 w-3.5 shrink-0" />
            <span className="truncate font-mono">{app.repo}</span>
            {app.auto && (
              <span className="rounded border border-emerald-500/40 px-1.5 py-0.5 font-mono text-[10px] tracking-wide text-emerald-500 uppercase">
                auto
              </span>
            )}
          </div>
        )}
        <a
          href={app.url}
          target="_blank"
          rel="noreferrer"
          className="text-muted-foreground hover:text-foreground truncate font-mono text-xs transition-colors"
        >
          {app.url.replace(/^https?:\/\//, '')}
        </a>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={() => onLogs(app.name)}>
            Logs
          </Button>
          <Button size="sm" variant="outline" onClick={() => onHistory(app.name)}>
            History
          </Button>
          <Button size="sm" variant="outline" onClick={() => onEnv(app.name)}>
            Env
          </Button>
          <Button size="sm" variant="ghost" onClick={() => onStop(app.name)}>
            Stop
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
