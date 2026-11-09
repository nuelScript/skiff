import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HardDrive, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { fmtBytes } from '@/lib/format'
import { useConfirm } from '@/providers/confirm-provider'
import { disksService, type Disk } from '@/services/api.service'

// DisksSection lists the added disks volumes and databases can live on, and lets the server's admin add one.
export function DisksSection() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const { data } = useQuery({ queryKey: queryKeys.disks, queryFn: () => disksService.list(), refetchInterval: 30000 })
  const [name, setName] = useState('')
  const [path, setPath] = useState('')
  const reload = () => qc.invalidateQueries({ queryKey: queryKeys.disks })
  const add = useMutation({
    mutationFn: () => disksService.add(name.trim(), path.trim()),
    onSuccess: () => {
      setName('')
      setPath('')
      reload()
    },
  })
  const remove = useMutation({ mutationFn: (id: string) => disksService.remove(id), onSuccess: reload })
  if (!data) return null
  if (data.disks.length === 0 && !data.admin) return null

  const ask = async (d: Disk) => {
    if (await confirm({ title: `Stop using ${d.name}?`, description: 'Nothing is deleted from it. Only a disk nothing is on can be removed.', confirmText: 'Remove' }))
      remove.mutate(d.id)
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (name.trim() && path.trim()) add.mutate()
  }

  const error = add.error ?? remove.error
  return (
    <section className="mt-6">
      <h2 className="text-muted-foreground mb-3 font-mono text-[11px] tracking-wider uppercase">Disks</h2>
      <div className="rounded-xl border border-foreground/8 bg-linear-to-b from-foreground/2 to-transparent p-5">
        <p className="text-muted-foreground text-sm leading-relaxed">
          Block storage added to this server, for volumes and databases that outgrow its own disk. Mount it first (your
          provider's guide shows how), then add the folder it's mounted at. New volumes and databases can go on it, and
          existing ones can move there from their own pages.
        </p>
        {data.disks.length > 0 && (
          <ul className="mt-4 divide-y divide-foreground/6 overflow-hidden rounded-lg border border-foreground/8">
            {data.disks.map((d) => {
              const used = d.total ? ((d.total - d.free) / d.total) * 100 : 0
              return (
                <li key={d.id} className="flex items-center gap-3 px-3 py-2.5 text-sm">
                  <HardDrive className="text-muted-foreground h-4 w-4 shrink-0" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate">
                      {d.name} <span className="text-muted-foreground font-mono text-xs">{d.path}</span>
                    </p>
                    {d.missing ? (
                      <p className="text-xs text-danger">Nothing is mounted there now. Volumes on it can't start until it's back.</p>
                    ) : (
                      <div className="mt-1 flex items-center gap-2">
                        <div className="h-1.5 w-32 overflow-hidden rounded-full bg-foreground/8">
                          <div className={'h-full ' + (used > 90 ? 'bg-danger' : used > 80 ? 'bg-warning' : 'bg-foreground/50')} style={{ width: `${Math.max(2, used)}%` }} />
                        </div>
                        <span className="text-muted-foreground text-xs">
                          {fmtBytes(d.free)} free of {fmtBytes(d.total)}
                        </span>
                      </div>
                    )}
                  </div>
                  {data.admin && (
                    <button
                      type="button"
                      aria-label={`Stop using ${d.name}`}
                      onClick={() => void ask(d)}
                      className="text-muted-foreground cursor-pointer p-1 transition-colors hover:text-danger"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  )}
                </li>
              )
            })}
          </ul>
        )}
        {data.admin && (
          <form onSubmit={submit} className="mt-4 grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto]">
            <Input aria-label="Disk name" placeholder="Name, e.g. data-1" value={name} onChange={(e) => { setName(e.target.value); add.reset() }} />
            <Input aria-label="Folder it's mounted at" className="font-mono" placeholder="/mnt/data" value={path} onChange={(e) => { setPath(e.target.value); add.reset() }} />
            <Button type="submit" size="sm" variant="outline" loading={add.isPending} disabled={!name.trim() || !path.trim()}>
              Add disk
            </Button>
          </form>
        )}
        {error && <p className="mt-2 text-xs text-danger">{errText(error, 'Something went wrong.')}</p>}
      </div>
    </section>
  )
}
