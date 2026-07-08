import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { authService } from '@/services/api.service'
import { errText } from '@/lib/errors'
import { RevealInput } from './ui'

export function DangerSection() {
  const [open, setOpen] = useState(false)
  const [password, setPassword] = useState('')
  const del = useMutation({
    mutationFn: () => authService.deleteAccount(password),
    onSuccess: () => {
      window.location.href = '/'
    },
  })

  return (
    <section className="rounded-xl border border-rose-500/20 bg-rose-500/3 p-5">
      <h2 className="text-sm font-semibold text-rose-200">Delete account</h2>
      <p className="text-muted-foreground mt-1 text-xs">
        Permanently remove your account. Personal teams with no projects, databases, or other members are
        removed too. This can't be undone.
      </p>
      {!open ? (
        <button
          onClick={() => setOpen(true)}
          className="mt-3 rounded-[6px] border border-rose-500/40 px-2.5 py-1 text-xs text-rose-300 transition hover:bg-rose-500/10"
        >
          Delete account
        </button>
      ) : (
        <div className="mt-3 max-w-sm space-y-2">
          <RevealInput
            placeholder="Confirm your password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => {
              setPassword(e.target.value)
              del.reset()
            }}
          />
          <div className="flex items-center gap-3">
            <button
              onClick={() => del.mutate()}
              disabled={del.isPending || !password}
              className="rounded-[6px] bg-rose-500/90 px-2.5 py-1 text-xs font-medium text-white transition hover:bg-rose-500 disabled:opacity-50"
            >
              {del.isPending ? 'Deleting…' : 'Permanently delete'}
            </button>
            <button
              onClick={() => {
                setOpen(false)
                setPassword('')
                del.reset()
              }}
              className="text-muted-foreground hover:text-foreground text-xs"
            >
              Cancel
            </button>
          </div>
        </div>
      )}
      {del.isError && (
        <p className="mt-2 text-xs text-rose-300">{errText(del.error, 'Could not delete your account.')}</p>
      )}
    </section>
  )
}
