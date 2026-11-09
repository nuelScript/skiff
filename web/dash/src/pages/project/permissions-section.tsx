import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ShieldAlert } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { describe, isEmpty, merge, splitList } from '@/lib/permissions'
import { useConfirm } from '@/providers/confirm-provider'
import { projectsService, type Permissions } from '@/services/api.service'

export function PermissionsSection({ app, onRedeploy }: { app: string; onRedeploy: () => void }) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const key = queryKeys.permissions(app)
  const { data } = useQuery({ queryKey: key, queryFn: () => projectsService.permissions(app) })
  const [caps, setCaps] = useState('')
  const [devices, setDevices] = useState('')
  const [sysctls, setSysctls] = useState('')
  const [privileged, setPrivileged] = useState(false)
  const [hostNetwork, setHostNetwork] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)

  const fill = (p: Permissions) => {
    setCaps((p.capabilities ?? []).join(', '))
    setDevices((p.devices ?? []).join(', '))
    setSysctls((p.sysctls ?? []).join(', '))
    setPrivileged(!!p.privileged)
    setHostNetwork(!!p.hostNetwork)
  }
  useEffect(() => {
    if (data) fill(data.granted)
  }, [data])

  if (!data) return null
  const granted = describe(data.granted)
  const wants = describe(data.wants)
  const missing = !isEmpty(data.wants) && JSON.stringify(merge(data.granted, data.wants)) !== JSON.stringify(merge(data.granted, {}))

  const save = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    const next: Permissions = {
      capabilities: splitList(caps),
      devices: splitList(devices),
      sysctls: splitList(sysctls),
      privileged,
      hostNetwork,
    }
    const strong = next.privileged || next.hostNetwork || next.capabilities?.some((c) => data.strong.includes(c.toUpperCase().replace(/^CAP_/, '')))
    if (
      !isEmpty(next) &&
      !(await confirm({
        title: strong ? `Give ${app} near-full control of the server?` : `Grant ${app} these permissions?`,
        description: strong
          ? "Privileged mode, the server's own network and capabilities like SYS_ADMIN let a container reach past its own walls, so anyone who controls this app could control the server. Only grant them to software you trust."
          : 'Each permission weakens the wall between this app and the server a little. They apply on the next deploy.',
        confirmText: 'Grant',
        destructive: true,
      }))
    )
      return
    setBusy(true)
    try {
      await projectsService.setPermissions(app, next)
      setSaved(true)
      qc.invalidateQueries({ queryKey: key })
    } catch (err) {
      setError(errText(err, 'Could not save the permissions.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-semibold">Machine permissions</h2>
        <p className="text-muted-foreground mt-1 text-xs">
          Some software needs more of the server than a container gets by default, like a VPN that needs NET_ADMIN and
          /dev/net/tun. Each permission is off until the server's admin grants it, and every change is in Activity.
        </p>
      </div>

      {wants.length > 0 && (
        <div className="border-warning/30 bg-warning/5 flex items-start justify-between gap-3 rounded-lg border p-3 text-xs leading-relaxed">
          <p>
            <strong className="font-semibold">Its Compose file asks for</strong> {wants.join('; ')}.
            {missing ? ' It runs without them until they are granted.' : ' All granted.'}
          </p>
          {data.canGrant && missing && (
            <Button size="sm" variant="outline" onClick={() => fill(merge(data.granted, data.wants))}>
              Fill these in
            </Button>
          )}
        </div>
      )}

      {data.canGrant ? (
        <form onSubmit={save} className="flex flex-col gap-3 rounded-xl border border-foreground/8 p-4">
          <FieldGroup className="gap-3">
            <Field>
              <FieldLabel htmlFor="perm-caps">Capabilities</FieldLabel>
              <Input id="perm-caps" className="font-mono" placeholder="Like NET_ADMIN" value={caps} onChange={(e) => setCaps(e.target.value)} />
            </Field>
            <Field>
              <FieldLabel htmlFor="perm-devices">Devices</FieldLabel>
              <Input id="perm-devices" className="font-mono" placeholder="Like /dev/net/tun" value={devices} onChange={(e) => setDevices(e.target.value)} />
            </Field>
            <Field>
              <FieldLabel htmlFor="perm-sysctls">Sysctls</FieldLabel>
              <Input id="perm-sysctls" className="font-mono" placeholder="Like net.ipv4.ip_forward=1" value={sysctls} onChange={(e) => setSysctls(e.target.value)} />
              <FieldDescription>Only ones that stay inside the app: net.*, kernel.shm*, kernel.msg*, kernel.sem and fs.mqueue.*.</FieldDescription>
            </Field>
            <label className="flex cursor-pointer items-start gap-2.5 text-sm">
              <input type="checkbox" className="mt-1" checked={privileged} onChange={(e) => setPrivileged(e.target.checked)} />
              <span>
                <span className="flex items-center gap-1.5 font-medium">
                  <ShieldAlert className="text-danger h-3.5 w-3.5" /> Privileged mode
                </span>
                <span className="text-muted-foreground block text-xs">
                  The escape hatch, for software that needs almost everything, like Docker-in-Docker. It gives the app
                  near-full control of the server.
                </span>
              </span>
            </label>
            <label className="flex cursor-pointer items-start gap-2.5 text-sm">
              <input type="checkbox" className="mt-1" checked={hostNetwork} onChange={(e) => setHostNetwork(e.target.checked)} />
              <span>
                <span className="flex items-center gap-1.5 font-medium">
                  <ShieldAlert className="text-danger h-3.5 w-3.5" /> The server's own network
                </span>
                <span className="text-muted-foreground block text-xs">
                  For software that needs to see the server's network, like some VPNs and discovery tools. The app runs one
                  copy, and a deploy stops the old version before starting the new one, so there's a short gap.
                </span>
              </span>
            </label>
          </FieldGroup>
          {error && <p className="text-xs text-danger">{error}</p>}
          <div className="flex items-center justify-end">
            <Button size="sm" type="submit" loading={busy}>
              Save permissions
            </Button>
          </div>
        </form>
      ) : (
        <div className="rounded-xl border border-foreground/8 p-4 text-xs">
          <p>{granted.length ? `Granted: ${granted.join('; ')}.` : 'None granted.'}</p>
          <p className="text-muted-foreground mt-1">Only the server's admin, an owner of the team that set it up, can change these.</p>
        </div>
      )}

      {saved && (
        <div className="border-warning/30 bg-warning/5 flex items-center justify-between gap-3 rounded-lg border p-3 text-xs">
          <span>Saved. Redeploy so the app's containers get them.</span>
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
