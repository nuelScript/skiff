import { useState } from 'react'
import { LogOut } from 'lucide-react'
import type { Me } from '@/services/api.service'

export default function UserMenu({
  me,
  logout,
}: {
  me: Me
  logout: () => void
}) {
  const [open, setOpen] = useState(false)
  const email = me.user?.email ?? ''
  const initial = (me.user?.name || email || '?').charAt(0).toUpperCase()

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="hover:bg-sidebar-accent flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left"
      >
        <span className="bg-accent text-foreground flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-medium">
          {initial}
        </span>
        <span className="text-muted-foreground min-w-0 flex-1 truncate text-sm">
          {email}
        </span>
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="border-border bg-popover absolute bottom-full left-3 z-50 mb-1 w-52 rounded-md border p-1 shadow-lg">
            <button
              onClick={() => {
                setOpen(false)
                void logout()
              }}
              className="hover:bg-accent flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm"
            >
              <LogOut className="h-3.5 w-3.5" /> Sign out
            </button>
          </div>
        </>
      )}
    </div>
  )
}
