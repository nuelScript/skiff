import { DiskPlace } from '@/components/disk-place'
import { disksService } from '@/services/api.service'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Archive, HardDrive, Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { fmtBytes } from '@/lib/format'
import { useConfirm } from '@/providers/confirm-provider'
import { projectsService, type Volumes } from '@/services/api.service'
import { volumeSchema, type VolumeInput } from '@/validations/volume'
import { VolumeBackupsDialog } from './volume-backups-dialog'

export function VolumesPanel({ app, onRedeploy }: { app: string; onRedeploy: () => void }) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const { data } = useQuery<Volumes>({
    queryKey: queryKeys.volumes(app),
    queryFn: () => projectsService.volumes(app),
    refetchInterval: 15000,
  })
  const volumes = data?.volumes ?? []
  const editable = data?.kind === 'project'
  const pending = volumes.some((v) => !v.attached)
  const [adding, setAdding] = useState(false)
  const [backups, setBackups] = useState('')
  const [budgetFor, setBudgetFor] = useState('')
  const [disk, setDisk] = useState('')
  const [shared, setShared] = useState('')
  const { data: sharedList } = useQuery({
    queryKey: queryKeys.sharedVolumes(app),
    queryFn: () => projectsService.sharedVolumes(app),
    enabled: adding,
  })
  const borrowed = sharedList?.find((v) => `${v.app}/${v.name}` === shared)
  const { data: disks } = useQuery({ queryKey: queryKeys.disks, queryFn: () => disksService.list(), staleTime: 60000 })
  const [budgetGB, setBudgetGB] = useState('')

  const saveBudget = async (name: string) => {
    setServerError('')
    const gb = budgetGB.trim() === '' ? 0 : Number(budgetGB)
    if (!Number.isFinite(gb) || gb < 0) {
      setServerError('Use a size in GB, or leave it empty for no budget.')
      return
    }
    try {
      await projectsService.setVolumeBudget(app, name, Math.round(gb * 1024 ** 3))
      setBudgetFor('')
      reload()
    } catch (err) {
      setServerError(errText(err, 'Could not set the budget.'))
    }
  }
  const [serverError, setServerError] = useState('')
  const reload = () => qc.invalidateQueries({ queryKey: queryKeys.volumes(app) })

  const form = useForm<VolumeInput>({
    resolver: zodResolver(volumeSchema),
    defaultValues: { name: 'data', path: '/data' },
  })

  const create = form.handleSubmit(async (values) => {
    setServerError('')
    try {
      await projectsService.addVolume(app, values.name, values.path, borrowed ? '' : disk, borrowed?.app)
      form.reset()
      setShared('')
      setAdding(false)
      reload()
    } catch (err) {
      setServerError(errText(err, 'Could not add the volume.'))
    }
  })

  const del = async (name: string, owner?: string) => {
    if (
      !(await confirm(
        owner
          ? {
              title: `Stop using ${owner}'s ${name} volume?`,
              description: `This app no longer mounts it after the next deploy. Its files stay with ${owner}.`,
              confirmText: 'Stop using',
            }
          : {
              title: `Delete the ${name} volume?`,
              description: 'Its files are deleted on the next deploy. This cannot be undone.',
              confirmText: 'Delete',
              destructive: true,
            },
      ))
    )
      return
    try {
      await projectsService.deleteVolume(app, name)
      reload()
    } catch (err) {
      setServerError(errText(err, 'Could not delete the volume.'))
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-end justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">Volumes</h2>
          <p className="text-muted-foreground mt-1 text-xs">
            {data?.kind === 'cli'
              ? "Directories that keep their files across deploys. This app's volumes come from the [[volumes]] section of its skiff.toml."
              : data?.kind === 'preview'
                ? "This preview mounts its project's volumes, on empty copies of its own — production's files are never touched."
                : "Directories that keep their files across deploys, rollbacks and restarts — for SQLite databases, uploads, caches and downloaded models. Shared by every replica, worker and job, and can be shared with the team's other projects; previews get empty ones of their own."}
          </p>
        </div>
        {editable && !adding && (
          <Button size="sm" onClick={() => setAdding(true)} className="shrink-0">
            <Plus />
            Add volume
          </Button>
        )}
      </div>

      {adding && (
        <form
          onSubmit={create}
          className="flex flex-col gap-3 rounded-xl border border-foreground/10 bg-linear-to-b from-foreground/2 to-transparent p-4"
        >
          <FieldGroup className="grid gap-3 sm:grid-cols-2">
            {(sharedList?.length ?? 0) > 0 && (
              <Field className="sm:col-span-2">
                <FieldLabel htmlFor="vol-source">Volume</FieldLabel>
                <select
                  id="vol-source"
                  value={shared}
                  onChange={(e) => {
                    setShared(e.target.value)
                    const v = sharedList?.find((s) => `${s.app}/${s.name}` === e.target.value)
                    form.reset(v ? { name: v.name, path: v.path } : { name: 'data', path: '/data' })
                  }}
                  className="h-9 cursor-pointer rounded-md border border-input bg-transparent px-2.5 text-sm"
                >
                  <option value="">A new, empty volume</option>
                  {sharedList?.map((v) => (
                    <option key={`${v.app}/${v.name}`} value={`${v.app}/${v.name}`}>
                      {v.app}'s {v.name} volume
                    </option>
                  ))}
                </select>
                {borrowed && (
                  <p className="text-muted-foreground text-xs">
                    Both apps see the same files, so a model {borrowed.app} has downloaded is ready here
                    without downloading it again.
                  </p>
                )}
              </Field>
            )}
            <Field data-invalid={!!form.formState.errors.name}>
              <FieldLabel htmlFor="vol-name">Name</FieldLabel>
              <Input
                id="vol-name"
                className="font-mono"
                placeholder="data"
                readOnly={!!borrowed}
                aria-invalid={!!form.formState.errors.name}
                {...form.register('name')}
              />
              <FieldError errors={[form.formState.errors.name]} />
            </Field>
            <Field data-invalid={!!form.formState.errors.path}>
              <FieldLabel htmlFor="vol-path">Mount path</FieldLabel>
              <Input
                id="vol-path"
                className="font-mono"
                placeholder="/data"
                aria-invalid={!!form.formState.errors.path}
                {...form.register('path')}
              />
              <FieldError errors={[form.formState.errors.path]} />
            </Field>
            {!borrowed && (disks?.disks.length ?? 0) > 0 && (
              <Field>
                <FieldLabel htmlFor="vol-disk">Keep its files on</FieldLabel>
                <select
                  id="vol-disk"
                  value={disk}
                  onChange={(e) => setDisk(e.target.value)}
                  className="h-9 cursor-pointer rounded-md border border-input bg-transparent px-2.5 text-sm"
                >
                  <option value="">The server's disk</option>
                  {disks?.disks.map((d) => (
                    <option key={d.id} value={d.id} disabled={d.missing}>
                      {d.name} ({d.path})
                    </option>
                  ))}
                </select>
              </Field>
            )}
          </FieldGroup>
          {serverError && <p className="text-xs text-danger">{serverError}</p>}
          <div className="flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={() => {
                setAdding(false)
                setServerError('')
                setShared('')
                form.reset()
              }}
              className="text-muted-foreground hover:text-foreground text-xs"
            >
              Cancel
            </button>
            <Button size="sm" type="submit" loading={form.formState.isSubmitting}>
              Add volume
            </Button>
          </div>
        </form>
      )}

      {volumes.length === 0 && !adding ? (
        <div className="text-muted-foreground rounded-xl border border-foreground/8 py-10 text-center text-sm">
          No volumes. Files the app writes are lost on each deploy.
        </div>
      ) : (
        volumes.length > 0 && (
          <div className="divide-y divide-foreground/5 overflow-hidden rounded-xl border border-foreground/8">
            {volumes.map((v) => (
              <div key={v.name} className="flex items-center gap-3 p-4">
                <HardDrive className="text-muted-foreground h-4 w-4 shrink-0" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{v.name}</p>
                  <p className="text-muted-foreground mt-0.5 truncate font-mono text-xs">{v.path}</p>
                  {(v.owner || (v.usedBy?.length ?? 0) > 0) && (
                    <p className="text-muted-foreground mt-0.5 truncate text-xs">
                      {v.owner ? `Shared from ${v.owner}` : `Also used by ${v.usedBy?.join(', ')}`}
                    </p>
                  )}
                </div>
                {budgetFor === v.name ? (
                  <form
                    onSubmit={(e) => {
                      e.preventDefault()
                      void saveBudget(v.name)
                    }}
                    className="flex shrink-0 items-center gap-1.5"
                  >
                    <Input
                      autoFocus
                      aria-label={`Budget for ${v.name} in GB`}
                      inputMode="decimal"
                      placeholder="GB"
                      className="h-7 w-20 font-mono text-xs"
                      value={budgetGB}
                      onChange={(e) => setBudgetGB(e.target.value)}
                    />
                    <Button size="sm" type="submit" variant="outline">
                      Save
                    </Button>
                  </form>
                ) : v.attached ? (
                  <button
                    type="button"
                    title={editable && !v.owner ? 'Set a size budget' : undefined}
                    disabled={!editable || !!v.owner}
                    onClick={() => {
                      setBudgetFor(v.name)
                      setBudgetGB(v.budget ? String(+(v.budget / 1024 ** 3).toFixed(2)) : '')
                    }}
                    className={
                      'shrink-0 cursor-pointer font-mono text-[11px] tabular-nums disabled:cursor-default ' +
                      (v.budget && v.size > v.budget ? 'text-danger' : 'text-muted-foreground hover:text-foreground')
                    }
                  >
                    {fmtBytes(v.size)}
                    {v.budget ? ` of ${fmtBytes(v.budget)}` : editable && !v.owner ? ' · set budget' : ''}
                  </button>
                ) : (
                  <span className="shrink-0 text-[11px] text-warning">Not mounted yet</span>
                )}
                {data?.kind !== 'preview' && !v.owner && (
                  <DiskPlace
                    target={{ app, volume: v.name }}
                    label={`the ${v.name} volume`}
                    disk={v.disk}
                    move={v.move}
                    canMove={editable}
                    invalidate={queryKeys.volumes(app)}
                    downtime={
                      v.usedBy?.length
                        ? `${[app, ...v.usedBy].join(', ')} stop while its files are copied, then deploy again on the moved volume.`
                        : `${app} stops while its files are copied, then deploys again on the moved volume.`
                    }
                  />
                )}
                {data?.kind !== 'preview' && v.attached && !v.owner && (
                  <button
                    onClick={() => setBackups(v.name)}
                    title="Backups"
                    className="text-muted-foreground hover:text-foreground p-1.5 transition"
                  >
                    <Archive className="h-3.5 w-3.5" />
                  </button>
                )}
                {editable && (
                  <button
                    onClick={() => del(v.name, v.owner)}
                    title={v.owner ? 'Stop using' : 'Delete'}
                    className="text-muted-foreground p-1.5 transition hover:text-danger"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>
            ))}
          </div>
        )
      )}
      {pending && editable && (
        <div className="flex items-center justify-between gap-3 rounded-xl border border-warning/20 bg-warning/5 px-4 py-3">
          <p className="text-xs text-warning">Redeploy to mount new volumes.</p>
          <Button size="sm" variant="outline" onClick={onRedeploy}>
            Redeploy
          </Button>
        </div>
      )}
      {serverError && !adding && <p className="text-xs text-danger">{serverError}</p>}
      <VolumeBackupsDialog
        app={app}
        volume={backups}
        open={backups !== ''}
        onOpenChange={(v) => !v && setBackups('')}
      />
    </div>
  )
}
