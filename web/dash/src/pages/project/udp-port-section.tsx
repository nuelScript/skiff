import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { useConfirm } from '@/providers/confirm-provider'
import { projectsService } from '@/services/api.service'

export function UDPPortSection({ app, onRedeploy }: { app: string; onRedeploy: () => void }) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const key = queryKeys.udpPort(app)
  const { data } = useQuery({ queryKey: key, queryFn: () => projectsService.udpPort(app) })
  const [value, setValue] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [changed, setChanged] = useState(false)

  const save = async (port: number) => {
    setError('')
    setBusy(true)
    try {
      await projectsService.setUDPPort(app, port)
      setValue('')
      setChanged(true)
      qc.invalidateQueries({ queryKey: key })
    } catch (err) {
      setError(errText(err, 'Could not change the port.'))
    } finally {
      setBusy(false)
    }
  }

  const open = async (e: FormEvent) => {
    e.preventDefault()
    const port = Number(value)
    const lowest = data?.lowPorts ? 1 : 1024
    if (!Number.isInteger(port) || port < lowest || port > 65535) {
      setError(
        port >= 1 && port < 1024
          ? "Ports under 1024 can only be opened by the server's admin, an owner of the team that set it up."
          : `Use a port from ${lowest} to 65535.`,
      )
      return
    }
    if (
      await confirm({
        title: `Open UDP port ${port} to the internet?`,
        description: `Anyone who can reach your server can send ${app} packets on this port. You may also need to allow UDP ${port} in your provider's firewall.`,
        confirmText: 'Open port',
        destructive: true,
      })
    )
      void save(port)
  }

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-semibold">Public UDP port</h2>
        <p className="text-muted-foreground mt-1 text-xs">
          For game servers, voice, WebRTC or a VPN like WireGuard. Your app listens on this UDP port, and Skiff opens the
          same number to the internet. Each client stays on one version until it goes quiet, so a deploy doesn't drop
          anyone mid-session.
        </p>
      </div>
      {data?.port ? (
        <div className="flex items-center justify-between gap-3 rounded-xl border border-foreground/8 p-4">
          <p className="font-mono text-sm">Open on UDP port {data.port}</p>
          <Button size="sm" variant="outline" loading={busy} onClick={() => void save(0)}>
            Close port
          </Button>
        </div>
      ) : (
        <form onSubmit={open} className="flex items-start gap-2">
          <Input
            aria-label="Public UDP port"
            inputMode="numeric"
            className="font-mono sm:max-w-40"
            placeholder="51820"
            value={value}
            onChange={(e) => setValue(e.target.value)}
          />
          <Button size="sm" type="submit" variant="outline" loading={busy} disabled={!value}>
            Open port
          </Button>
        </form>
      )}
      {changed && (
        <div className="border-warning/30 bg-warning/5 flex items-center justify-between gap-3 rounded-lg border p-3 text-xs">
          <span>Redeploy so the app's container gets the change.</span>
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
