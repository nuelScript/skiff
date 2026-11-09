import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { projectsService, type EgressMode } from '@/services/api.service'

const MODES: { value: EgressMode; label: string; detail: string }[] = [
  { value: '', label: 'Anywhere', detail: 'It can reach the internet, like any server.' },
  {
    value: 'private',
    label: "Only the team's own services",
    detail: 'Its apps and databases, and nothing on the internet.',
  },
  { value: 'list', label: 'Those, and hosts you list', detail: 'For an API it calls, like api.openai.com.' },
]

export function EgressSection({ app, onRedeploy }: { app: string; onRedeploy: () => void }) {
  const qc = useQueryClient()
  const key = queryKeys.egress(app)
  const { data } = useQuery({ queryKey: key, queryFn: () => projectsService.egress(app) })
  const [mode, setMode] = useState<EgressMode>('')
  const [allow, setAllow] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (!data) return
    setMode(data.policy.mode ?? '')
    setAllow((data.policy.allow ?? []).join('\n'))
  }, [data])

  if (!data) return null
  const save = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await projectsService.setEgress(app, {
        mode,
        allow: mode === 'list' ? allow.split(/[\s,]+/).filter(Boolean) : [],
      })
      setSaved(true)
      qc.invalidateQueries({ queryKey: key })
    } catch (err) {
      setError(errText(err, 'Could not save.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-semibold">Outbound network</h2>
        <p className="text-muted-foreground mt-1 text-xs">
          Where the app may connect. Limiting it means code that goes wrong, or code you didn't write, like an agent's,
          can't send your data somewhere else.
        </p>
      </div>
      {!data.available ? (
        <p className="rounded-xl border border-foreground/8 p-4 text-xs">
          This server can't limit outbound connections: it needs Linux, with Skiff running as root, as the installer sets
          up.
        </p>
      ) : (
        <form onSubmit={save} className="flex flex-col gap-3 rounded-xl border border-foreground/8 p-4">
          <fieldset className="grid gap-2">
            <legend className="sr-only">Outbound network</legend>
            {MODES.map((m) => (
              <label key={m.value || 'any'} className="flex cursor-pointer items-start gap-2.5 text-sm">
                <input
                  type="radio"
                  name={`egress-${app}`}
                  className="mt-1"
                  checked={mode === m.value}
                  onChange={() => setMode(m.value)}
                />
                <span>
                  <span className="font-medium">{m.label}</span>
                  <span className="text-muted-foreground block text-xs">{m.detail}</span>
                </span>
              </label>
            ))}
          </fieldset>
          {mode === 'list' && (
            <Textarea
              aria-label="Hosts it may reach"
              className="font-mono text-xs"
              rows={3}
              placeholder={'api.openai.com\n203.0.113.0/24'}
              value={allow}
              onChange={(e) => setAllow(e.target.value)}
            />
          )}
          {error && <p className="text-xs text-danger">{error}</p>}
          <div className="flex items-center justify-end">
            <Button size="sm" type="submit" loading={busy}>
              Save
            </Button>
          </div>
        </form>
      )}
      {saved && (
        <div className="border-warning/30 bg-warning/5 flex items-center justify-between gap-3 rounded-lg border p-3 text-xs">
          <span>Redeploy so the app's containers get it.</span>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setSaved(false)
              onRedeploy()
            }}
          >
            Redeploy now
          </Button>
        </div>
      )}
    </section>
  )
}
