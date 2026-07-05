import { useState } from 'react'
import { Check, ChevronsUpDown } from 'lucide-react'
import type { Me } from '@/services/api.service'

export default function TeamSwitcher({
  me,
  switchTeam,
  onMembers,
  onCreate,
}: {
  me: Me
  switchTeam: (id: string) => Promise<void>
  onMembers: () => void
  onCreate: () => void
}) {
  const [open, setOpen] = useState(false)
  const current = me.teams?.find((t) => t.id === me.team)

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="hover:bg-accent flex items-center gap-2 rounded-md border px-3 py-1.5 text-sm"
      >
        <span className="max-w-[160px] truncate">{current?.name ?? 'Team'}</span>
        <ChevronsUpDown className="text-muted-foreground h-3.5 w-3.5" />
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="border-border bg-popover absolute left-0 z-50 mt-1 w-60 rounded-md border p-1 shadow-lg">
            <p className="text-muted-foreground px-2 py-1.5 font-mono text-[10px] tracking-wide uppercase">
              Teams
            </p>
            {me.teams?.map((t) => (
              <button
                key={t.id}
                onClick={() => {
                  setOpen(false)
                  if (t.id !== me.team) void switchTeam(t.id)
                }}
                className="hover:bg-accent flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-sm"
              >
                <span className="truncate">{t.name}</span>
                {t.id === me.team && <Check className="h-3.5 w-3.5 shrink-0" />}
              </button>
            ))}
            <div className="bg-border my-1 h-px" />
            <button
              onClick={() => {
                setOpen(false)
                onMembers()
              }}
              className="hover:bg-accent w-full rounded px-2 py-1.5 text-left text-sm"
            >
              Members
            </button>
            <button
              onClick={() => {
                setOpen(false)
                onCreate()
              }}
              className="hover:bg-accent text-muted-foreground w-full rounded px-2 py-1.5 text-left text-sm"
            >
              Create team…
            </button>
          </div>
        </>
      )}
    </div>
  )
}
