import { useEffect, useState } from 'react'
import { Trash2, ScrollText } from 'lucide-react'
import { useApps } from '@/hooks/use-apps'
import { useLogStream } from '@/hooks/use-log-stream'
import { useAutoScroll } from '@/hooks/use-auto-scroll'

const runningDot = (state: string): string =>
  state === 'running'
    ? 'bg-emerald-400'
    : state === 'exited' || state === 'missing'
      ? 'bg-rose-400'
      : 'bg-amber-400'

export default function LogsPage() {
  const { apps } = useApps()
  const [selected, setSelected] = useState<string | null>(null)

  // Default to the first app once they load; keep the selection valid.
  useEffect(() => {
    if (apps.length === 0) return
    setSelected((cur) => (cur && apps.some((a) => a.name === cur) ? cur : apps[0].name))
  }, [apps])

  const { lines, live, clear } = useLogStream(selected)
  const scrollRef = useAutoScroll<HTMLDivElement>(lines.length)

  return (
    <div className="flex h-[calc(100svh-3.5rem)] flex-col overflow-hidden px-8 py-8">
      <header className="mb-6 shrink-0">
        <h1 className="text-xl font-semibold tracking-tight">Logs</h1>
        <p className="text-muted-foreground mt-1 text-sm">
          Live runtime output streamed straight from the container.
        </p>
      </header>

      <div className="flex min-h-0 flex-1 gap-5">
        {/* app picker */}
        <aside className="w-52 shrink-0">
          <p className="text-muted-foreground mb-2 px-1 font-mono text-[10px] tracking-wider uppercase">
            Apps
          </p>
          {apps.length === 0 ? (
            <p className="text-muted-foreground px-1 text-xs">No apps deployed.</p>
          ) : (
            <div className="flex flex-col gap-0.5">
              {apps.map((a) => (
                <button
                  key={a.name}
                  onClick={() => setSelected(a.name)}
                  className={
                    'flex items-center gap-2.5 rounded-[6px] px-2.5 py-1.5 text-left text-sm transition-colors ' +
                    (selected === a.name
                      ? 'bg-white/8 text-foreground'
                      : 'text-muted-foreground hover:bg-white/3 hover:text-foreground')
                  }
                >
                  <span className={'h-1.5 w-1.5 shrink-0 rounded-full ' + runningDot(a.state)} />
                  <span className="truncate font-mono text-xs">{a.name}</span>
                </button>
              ))}
            </div>
          )}
        </aside>

        {/* terminal */}
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-xl border border-white/10 bg-black/60">
          <div className="flex h-10 shrink-0 items-center gap-3 border-b border-white/6 px-4">
            <div className="flex items-center gap-1.5">
              <span className="h-2.5 w-2.5 rounded-full border border-white/15" />
              <span className="h-2.5 w-2.5 rounded-full border border-white/15" />
              <span className="h-2.5 w-2.5 rounded-full border border-white/15" />
            </div>
            <span className="text-muted-foreground font-mono text-xs">
              {selected ? selected : 'no app selected'}
            </span>
            {selected && (
              <span className="flex items-center gap-1.5 font-mono text-[11px]">
                <span
                  className={'h-1.5 w-1.5 rounded-full ' + (live ? 'bg-emerald-400 pulse-dot' : 'bg-white/25')}
                />
                <span className={live ? 'text-emerald-300/80' : 'text-muted-foreground'}>
                  {live ? 'live' : 'ended'}
                </span>
              </span>
            )}
            <div className="ml-auto flex items-center gap-3">
              <span className="text-muted-foreground font-mono text-[11px] tabular-nums">
                {lines.length} lines
              </span>
              <button
                onClick={clear}
                disabled={lines.length === 0}
                className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-xs transition-colors disabled:opacity-30"
              >
                <Trash2 className="h-3.5 w-3.5" />
                Clear
              </button>
            </div>
          </div>

          <div
            ref={scrollRef}
            className="term-scroll min-h-0 flex-1 overflow-auto px-4 py-3 font-mono text-xs leading-relaxed"
          >
            {!selected ? (
              <div className="text-muted-foreground flex h-full flex-col items-center justify-center gap-2">
                <ScrollText className="h-6 w-6 opacity-40" />
                <span>Select an app to stream its logs.</span>
              </div>
            ) : lines.length === 0 ? (
              <p className="text-muted-foreground">Waiting for output…</p>
            ) : (
              lines.map((line, i) => (
                <div key={i} className="text-white/70 whitespace-pre-wrap wrap-break-word">
                  {line || ' '}
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
