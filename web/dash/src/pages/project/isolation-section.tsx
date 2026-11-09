import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { projectsService } from '@/services/api.service'

export function IsolationSection({ app, onRedeploy }: { app: string; onRedeploy: () => void }) {
  const qc = useQueryClient()
  const key = queryKeys.isolation(app)
  const { data } = useQuery({ queryKey: key, queryFn: () => projectsService.isolation(app) })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [changed, setChanged] = useState(false)
  if (!data) return null
  const strong = data.isolation === 'strong'

  const set = async (isolation: '' | 'strong') => {
    setError('')
    setBusy(true)
    try {
      await projectsService.setIsolation(app, isolation)
      setChanged(true)
      qc.invalidateQueries({ queryKey: key })
    } catch (err) {
      setError(errText(err, 'Could not change the isolation.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-semibold">Isolation</h2>
        <p className="text-muted-foreground mt-1 text-xs">
          For code you don't fully trust, like an agent's or a customer's, strong isolation runs the app under gVisor,
          which gives it a kernel of its own instead of the server's. It's slower at heavy disk and network work.
        </p>
      </div>
      <div className="flex items-center justify-between gap-3 rounded-xl border border-foreground/8 p-4">
        <p className="text-sm">{strong ? 'Strong: runs under gVisor' : "Standard: Docker's usual isolation"}</p>
        {strong ? (
          <Button size="sm" variant="outline" loading={busy} onClick={() => void set('')}>
            Use standard
          </Button>
        ) : (
          <Button
            size="sm"
            variant="outline"
            loading={busy}
            disabled={!data.available}
            title={data.available ? undefined : 'gVisor isn’t installed on this server yet'}
            onClick={() => void set('strong')}
          >
            Use strong
          </Button>
        )}
      </div>
      {!data.available && !strong && (
        <p className="text-muted-foreground text-xs">
          gVisor isn't installed on this server yet. The docs explain how to add it.
        </p>
      )}
      {changed && (
        <div className="border-warning/30 bg-warning/5 flex items-center justify-between gap-3 rounded-lg border p-3 text-xs">
          <span>Redeploy so the app's containers get it.</span>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setChanged(false)
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
