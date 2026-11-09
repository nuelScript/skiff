import { useQuery } from '@tanstack/react-query'
import { Cpu } from 'lucide-react'
import { queryKeys } from '@/constants/query-keys'
import { projectsService } from '@/services/api.service'

// GPUsSection shows the server's GPUs: how busy each is, its memory and temperature, and which app has it. It hides itself on a server without one.
export function GPUsSection() {
  const { data } = useQuery({ queryKey: queryKeys.serverGPUs, queryFn: () => projectsService.gpus(), refetchInterval: 15000 })
  if (!data?.gpus.length) return null
  return (
    <section className="mt-6">
      <h2 className="text-muted-foreground mb-3 font-mono text-[11px] tracking-wider uppercase">GPUs · {data.gpus.length}</h2>
      <div className="rounded-xl border border-foreground/8 bg-linear-to-b from-foreground/2 to-transparent p-5">
        {data.problem && <p className="mb-3 text-xs text-warning">{data.problem}</p>}
        <ul className="divide-y divide-foreground/6 overflow-hidden rounded-lg border border-foreground/8">
          {data.gpus.map((g) => {
            const mem = g.memoryTotal ? (g.memoryUsed / g.memoryTotal) * 100 : 0
            return (
              <li key={g.uuid} className="grid gap-2 px-3 py-2.5 text-sm sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-center">
                <span className="flex min-w-0 items-center gap-2">
                  <Cpu className="text-muted-foreground h-4 w-4 shrink-0" />
                  <span className="truncate">
                    {g.index} · {g.name}
                  </span>
                </span>
                <span className="text-muted-foreground text-xs">{g.utilization}% busy</span>
                <span className="text-muted-foreground text-xs">
                  {(g.memoryUsed / 1024).toFixed(1)} of {(g.memoryTotal / 1024).toFixed(0)} GB ({Math.round(mem)}%)
                  {' · '}
                  {g.temperature}°C
                </span>
                <span className={'text-xs ' + (g.app ? 'font-mono' : 'text-muted-foreground')}>{g.app || 'free'}</span>
              </li>
            )
          })}
        </ul>
      </div>
    </section>
  )
}
