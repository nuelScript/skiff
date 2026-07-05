import type { Deploy } from '@/services/api.service'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

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

export function DeployHistory({
  app,
  deploys,
  onClose,
  onViewLog,
}: {
  app: string | null
  deploys: Deploy[]
  onClose: () => void
  onViewLog: (app: string, id: string) => void
}) {
  return (
    <Dialog open={!!app} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            Deploys · <span className="font-mono">{app}</span>
          </DialogTitle>
        </DialogHeader>
        <div className="max-h-80 overflow-auto rounded-md border">
          {deploys.length === 0 ? (
            <p className="text-muted-foreground p-6 text-center text-sm">
              No deploys yet.
            </p>
          ) : (
            deploys.map((d) => (
              <button
                key={d.id}
                onClick={() => app && onViewLog(app, d.id)}
                className="hover:bg-accent flex w-full items-center justify-between border-b px-4 py-3 text-left last:border-0"
              >
                <span className="flex items-center gap-2.5">
                  <span className={'h-1.5 w-1.5 rounded-full ' + statusDot(d.status)} />
                  <span className="font-mono text-xs">
                    {d.commit || d.status}
                  </span>
                </span>
                <span className="text-muted-foreground font-mono text-[11px]">
                  {d.trigger} · {rel(d.started)}
                </span>
              </button>
            ))
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
