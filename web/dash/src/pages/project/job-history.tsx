import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight } from 'lucide-react'
import { queryKeys } from '@/constants/query-keys'
import { fmtDuration, relTime } from '@/lib/format'
import { runStatusLabel, runStatusTone, runTrigger } from '@/lib/jobs'
import { projectsService } from '@/services/api.service'
import { StepResults } from './job-steps'

export function JobHistory({ id, live }: { id: string; live: boolean }) {
  const { data: runs, isPending } = useQuery({
    queryKey: queryKeys.jobRuns(id),
    queryFn: () => projectsService.jobRuns(id),
    refetchInterval: live ? 3000 : false,
  })
  const [open, setOpen] = useState<number | null>(null)

  if (isPending) return <p className="text-muted-foreground py-3 text-xs">Loading runs…</p>
  if (!runs?.length) return <p className="text-muted-foreground py-3 text-xs">No runs yet.</p>

  return (
    <ul className="divide-y divide-foreground/5 overflow-hidden rounded-lg border border-foreground/8">
      {runs.map((r) => {
        const expanded = open === r.id
        const took = r.finished >= r.started && r.finished > 0 ? fmtDuration(r.finished - r.started) : ''
        return (
          <li key={r.id}>
            <button
              type="button"
              onClick={() => setOpen(expanded ? null : r.id)}
              aria-expanded={expanded}
              className="flex w-full cursor-pointer items-center gap-3 px-3 py-2 text-left text-xs transition-colors hover:bg-foreground/[0.03]"
            >
              <ChevronRight
                className={'text-muted-foreground h-3.5 w-3.5 shrink-0 transition-transform ' + (expanded ? 'rotate-90' : '')}
              />
              <span className={'w-20 shrink-0 font-medium ' + runStatusTone[r.status]}>
                {runStatusLabel[r.status]}
              </span>
              <span className="text-muted-foreground min-w-0 flex-1 truncate">
                {relTime(r.started)} · {runTrigger(r)}
                {r.status === 'failed' && r.exitCode > 0 ? ` · exit ${r.exitCode}` : ''}
              </span>
              {took && <span className="text-muted-foreground shrink-0 font-mono text-[11px]">{took}</span>}
            </button>
            {expanded && r.steps && r.steps.length > 0 && <StepResults steps={r.steps} />}
            {expanded && (
              <pre className="mx-3 mb-3 max-h-64 overflow-auto rounded-md border border-foreground/8 bg-inset p-2.5 font-mono text-[11px] whitespace-pre-wrap">
                {r.output.trim() || 'No output.'}
              </pre>
            )}
          </li>
        )
      })}
    </ul>
  )
}
