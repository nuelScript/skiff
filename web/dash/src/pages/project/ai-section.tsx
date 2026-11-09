import { useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { fmtCount } from '@/lib/format'
import { aiService } from '@/services/api.service'

export function AISection({ app, onRedeploy }: { app: string; onRedeploy: () => void }) {
  const qc = useQueryClient()
  const key = queryKeys.projectAI(app)
  const { data } = useQuery({ queryKey: key, queryFn: () => aiService.project(app), refetchInterval: 30000 })
  const [budget, setBudget] = useState('')
  const [error, setError] = useState('')
  const [turnedOn, setTurnedOn] = useState(false)

  const set = useMutation({
    mutationFn: (v: { enabled: boolean; budget: number }) => aiService.setProject(app, v.enabled, v.budget),
    onSuccess: (_, v) => {
      setBudget('')
      setTurnedOn(v.enabled && !data?.enabled)
      qc.invalidateQueries({ queryKey: key })
    },
    onError: (err) => setError(errText(err, 'Could not change AI access.')),
  })

  const parseBudget = () => {
    const n = budget.trim() === '' ? 0 : Number(budget.replace(/[,_\s]/g, ''))
    if (!Number.isInteger(n) || n < 0) {
      setError('Give the budget as a whole number of tokens, or leave it blank for no limit.')
      return null
    }
    return n
  }

  const submit = (e: FormEvent, enabled: boolean) => {
    e.preventDefault()
    setError('')
    const n = budget.trim() === '' && data?.enabled ? data.budget : parseBudget()
    if (n !== null) set.mutate({ enabled, budget: n })
  }

  if (!data) return null
  const used = data.used.input + data.used.output
  const share = data.budget ? Math.min(1, used / data.budget) : 0
  const provider = data.providers[0] ?? 'openai'

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-semibold">AI access</h2>
        <p className="text-muted-foreground mt-1 text-xs">
          Let this app call your team's AI providers through Skiff. It gets <code className="font-mono">SKIFF_AI_URL</code>{' '}
          and its own <code className="font-mono">SKIFF_AI_TOKEN</code>; the provider's key never enters the app, and
          Skiff stops its calls at the monthly budget.
        </p>
      </div>

      {data.providers.length === 0 && !data.enabled ? (
        <p className="rounded-xl border border-foreground/8 p-4 text-xs">
          Your team hasn't added an AI provider yet.{' '}
          <Link to="/settings?tab=integrations" className="cursor-pointer underline underline-offset-2">
            Add one in Settings → Integrations
          </Link>
          .
        </p>
      ) : data.enabled ? (
        <div className="flex flex-col gap-3 rounded-xl border border-foreground/8 p-4">
          <div className="flex flex-wrap items-baseline justify-between gap-2 text-sm">
            <span>
              <span className="font-mono">{fmtCount(used)}</span> tokens this month
              {data.budget > 0 && <span className="text-muted-foreground"> of {fmtCount(data.budget)}</span>}
            </span>
            <span className="text-muted-foreground text-xs">
              {data.used.requests.toLocaleString()} {data.used.requests === 1 ? 'call' : 'calls'} ·{' '}
              {fmtCount(data.used.input)} in, {fmtCount(data.used.output)} out
            </span>
          </div>
          {data.budget > 0 && (
            <div className="h-1.5 overflow-hidden rounded-full bg-foreground/8" role="presentation">
              <div
                className={share >= 1 ? 'h-full bg-danger' : share >= 0.8 ? 'h-full bg-warning' : 'h-full bg-foreground/50'}
                style={{ width: `${share * 100}%` }}
              />
            </div>
          )}
          {share >= 1 && (
            <p className="text-xs text-danger">The budget is used up, so Skiff is refusing this app's model calls until next month.</p>
          )}
          <form onSubmit={(e) => submit(e, true)} className="flex flex-wrap items-start gap-2">
            <Input
              aria-label="Monthly token budget"
              inputMode="numeric"
              className="font-mono sm:max-w-48"
              placeholder={data.budget ? `${data.budget.toLocaleString()} tokens` : 'No limit'}
              value={budget}
              onChange={(e) => setBudget(e.target.value)}
            />
            <Button size="sm" type="submit" variant="outline" loading={set.isPending} disabled={!budget.trim()}>
              Set budget
            </Button>
            <Button
              size="sm"
              type="button"
              variant="ghost"
              className="ml-auto"
              loading={set.isPending}
              onClick={() => set.mutate({ enabled: false, budget: 0 })}
            >
              Turn off
            </Button>
          </form>
          <pre className="overflow-x-auto rounded-lg bg-inset p-3 font-mono text-[11px] leading-relaxed">
            {`client = OpenAI(
    base_url=os.environ["SKIFF_AI_URL"] + "/${provider}/v1",
    api_key=os.environ["SKIFF_AI_TOKEN"],
)`}
          </pre>
          <p className="text-muted-foreground text-xs">
            Each provider is at <code className="font-mono">$SKIFF_AI_URL/&lt;its name&gt;</code>:{' '}
            {data.providers.map((p) => (
              <code key={p} className="mr-1.5 font-mono">
                {p}
              </code>
            ))}
          </p>
        </div>
      ) : (
        <form onSubmit={(e) => submit(e, true)} className="flex flex-wrap items-start gap-2">
          <Input
            aria-label="Monthly token budget"
            inputMode="numeric"
            className="font-mono sm:max-w-48"
            placeholder="Tokens a month"
            value={budget}
            onChange={(e) => setBudget(e.target.value)}
          />
          <Button size="sm" type="submit" variant="outline" loading={set.isPending}>
            Give AI access
          </Button>
          <p className="text-muted-foreground w-full text-xs">A budget like 2000000 stops it there each month. Leave it blank for no limit.</p>
        </form>
      )}

      {!data.gateway && (
        <p className="text-xs text-warning">
          Skiff couldn't find Docker's network on this server, so apps can't reach the AI gateway yet.
        </p>
      )}
      {turnedOn && (
        <div className="border-warning/30 bg-warning/5 flex items-center justify-between gap-3 rounded-lg border p-3 text-xs">
          <span>Redeploy so the app gets its token.</span>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setTurnedOn(false)
              onRedeploy()
            }}
          >
            Redeploy now
          </Button>
        </div>
      )}
      {error && <p className="text-xs text-danger">{error}</p>}
    </section>
  )
}
