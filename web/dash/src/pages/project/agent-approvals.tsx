import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { relTime } from '@/lib/format'
import { aiService, type Approval } from '@/services/api.service'

const verdict: Record<Approval['status'], string> = {
  pending: 'waiting',
  approved: 'approved',
  denied: 'denied',
  expired: 'expired unanswered',
}

// AgentApprovals puts the agent's questions first: what it wants to do, waiting on a person.
export function AgentApprovals({ app }: { app: string }) {
  const { data } = useQuery({ queryKey: queryKeys.approvals(app), queryFn: () => aiService.approvals(app), refetchInterval: 5000 })
  const approvals = data?.approvals ?? []
  const pending = approvals.filter((a) => a.status === 'pending')
  const answered = approvals.filter((a) => a.status !== 'pending').slice(0, 5)
  if (approvals.length === 0) return null

  return (
    <div className="flex min-w-0 flex-col gap-3">
      {pending.length > 0 && (
        <div className="border-warning/30 bg-warning/5 flex flex-col gap-3 rounded-lg border p-3">
          <h3 className="text-xs font-medium">
            {pending.length === 1 ? 'It is asking before it acts' : `It is asking about ${pending.length} things before it acts`}
          </h3>
          {pending.map((a) => (
            <PendingApproval key={a.id} app={app} a={a} />
          ))}
        </div>
      )}
      {answered.length > 0 && (
        <details className="text-xs">
          <summary className="text-muted-foreground cursor-pointer">Earlier requests</summary>
          <ul className="mt-2 divide-y divide-foreground/6 rounded-lg border border-foreground/8">
            {answered.map((a) => (
              <li key={a.id} className="flex items-start justify-between gap-3 px-3 py-2">
                <div className="min-w-0">
                  <p className="truncate">{a.action}</p>
                  {a.note && <p className="text-muted-foreground truncate">“{a.note}”</p>}
                </div>
                <div className="shrink-0 text-right">
                  <p className={a.status === 'approved' ? 'text-success' : 'text-muted-foreground'}>{verdict[a.status]}</p>
                  <p className="text-muted-foreground">
                    {a.by && `${a.by} · `}
                    {relTime(a.answered || a.created)}
                  </p>
                </div>
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  )
}

function PendingApproval({ app, a }: { app: string; a: Approval }) {
  const qc = useQueryClient()
  const [note, setNote] = useState('')
  const answer = useMutation({
    mutationFn: (approve: boolean) => aiService.answerApproval(app, a.id, approve, note.trim()),
    onSettled: () => qc.invalidateQueries({ queryKey: queryKeys.approvals(app) }),
  })
  return (
    <div className="flex flex-col gap-2 rounded-md border border-foreground/8 bg-background/40 p-3">
      <div className="flex items-start justify-between gap-3">
        <p className="text-sm font-medium">{a.action}</p>
        <span className="text-muted-foreground shrink-0 text-xs">{relTime(a.created)}</span>
      </div>
      {a.detail && <p className="text-muted-foreground text-xs whitespace-pre-wrap">{a.detail}</p>}
      <div className="flex flex-wrap items-center gap-2">
        <Input
          aria-label="Note for the agent"
          className="h-8 min-w-0 flex-1 text-xs"
          placeholder="Note for the agent (optional)"
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
        <Button size="sm" variant="outline" loading={answer.isPending && answer.variables === false} onClick={() => answer.mutate(false)}>
          <X className="h-3.5 w-3.5" />
          Deny
        </Button>
        <Button size="sm" loading={answer.isPending && answer.variables === true} onClick={() => answer.mutate(true)}>
          <Check className="h-3.5 w-3.5" />
          Approve
        </Button>
      </div>
      {answer.error && <p className="text-xs text-danger">{errText(answer.error, 'Could not record the answer.')}</p>}
    </div>
  )
}
