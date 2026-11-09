import { useMutation, useQuery, useQueryClient, type QueryKey } from '@tanstack/react-query'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { useConfirm } from '@/providers/confirm-provider'
import { disksService, type MoveState } from '@/services/api.service'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const SERVER = 'server'

// DiskPlace says which disk a volume or database keeps its files on, and moves it to another. It hides itself on a server with no added disks.
export function DiskPlace({
  target,
  label,
  disk,
  move,
  canMove,
  invalidate,
  downtime,
}: {
  target: { database: string } | { app: string; volume: string }
  label: string
  disk?: string
  move?: MoveState
  canMove: boolean
  invalidate: QueryKey
  downtime: string
}) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const { data } = useQuery({ queryKey: queryKeys.disks, queryFn: () => disksService.list(), staleTime: 60000 })
  const go = useMutation({
    mutationFn: (to: string) => disksService.move(target, to),
    onSettled: () => qc.invalidateQueries({ queryKey: invalidate }),
  })
  const disks = data?.disks ?? []
  if (disks.length === 0 && !disk) return null

  const name = (id?: string) => (id ? (disks.find((d) => d.id === id)?.name ?? 'an added disk') : "the server's disk")
  if (move?.moving) {
    return <span className="shrink-0 text-[11px] text-warning">Moving…</span>
  }

  const choose = async (value: string) => {
    const to = value === SERVER ? '' : value
    if (to === (disk ?? '')) return
    if (
      await confirm({
        title: `Move ${label} to ${name(to)}?`,
        description: `${downtime} Skiff copies the files, checks the copy has every file and byte, and only then removes the old ones. If anything goes wrong, it stays where it is.`,
        confirmText: 'Move',
      })
    )
      go.mutate(to)
  }

  return (
    <div className="flex shrink-0 flex-col items-end gap-1">
      {canMove ? (
        <Select value={disk || SERVER} onValueChange={(v) => void choose(v)}>
          <SelectTrigger size="sm" aria-label={`Where ${label} keeps its files`} className="h-7 cursor-pointer text-[11px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={SERVER}>Server's disk</SelectItem>
            {disks.map((d) => (
              <SelectItem key={d.id} value={d.id} disabled={d.missing}>
                {d.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : (
        <span className="text-muted-foreground text-[11px]">on {name(disk)}</span>
      )}
      {(move?.error || go.error) && (
        <span className="max-w-56 text-right text-[11px] text-danger">
          {move?.error ? `The last move failed: ${move.error}` : errText(go.error, 'Could not start the move.')}
        </span>
      )}
    </div>
  )
}
