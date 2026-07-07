import { useState, type FormEvent } from 'react'
import { Trash2, X } from 'lucide-react'
import { authService, type Member } from '@/services/api.service'
import { errText } from '@/lib/errors'
import { Button } from '@/components/ui/button'
import { useConfirm } from '@/providers/confirm-provider'
import { Section, inputCls } from './ui'

export function MembersSection({
  members,
  meId,
  isOwner,
  onChange,
}: {
  members: Member[]
  meId: string
  isOwner: boolean
  onChange: () => void
}) {
  const [email, setEmail] = useState('')
  const [link, setLink] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const confirm = useConfirm()

  const invite = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setLink('')
    setBusy(true)
    try {
      const r = await authService.invite(email.trim(), 'member')
      setLink(r.link)
      setEmail('')
      onChange()
    } catch (err) {
      setError(errText(err, 'Could not create an invite.'))
    } finally {
      setBusy(false)
    }
  }

  const remove = async (m: Member) => {
    if (
      !(await confirm({
        title: `Remove ${m.user.name}?`,
        description: 'They lose access to this team.',
        confirmText: 'Remove',
        destructive: true,
      }))
    )
      return
    try {
      await authService.removeMember(m.user.id)
      onChange()
    } catch (err) {
      setError(errText(err, 'Could not remove that member.'))
    }
  }

  return (
    <Section title="Members" description="Everyone here can see and manage this team's projects.">
      <div className="divide-y divide-white/5 overflow-hidden rounded-lg border border-white/8">
        {members.map((m) => (
          <div key={m.user.id} className="flex items-center gap-3 px-3.5 py-2.5">
            <span className="grid h-8 w-8 shrink-0 place-items-center rounded-full border border-white/10 bg-white/5 text-xs font-medium uppercase">
              {m.user.name.charAt(0)}
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm">
                {m.user.name}
                {m.user.id === meId && <span className="text-muted-foreground"> (you)</span>}
              </p>
              <p className="text-muted-foreground truncate text-xs">{m.user.email}</p>
            </div>
            <span
              className={
                'rounded-full border px-2 py-0.5 font-mono text-[10px] uppercase ' +
                (m.role === 'owner'
                  ? 'border-white/15 text-foreground/80'
                  : 'text-muted-foreground border-white/10')
              }
            >
              {m.role}
            </span>
            {isOwner && m.user.id !== meId && (
              <button
                onClick={() => remove(m)}
                className="text-muted-foreground p-1 transition hover:text-rose-300"
                title={`Remove ${m.user.name}`}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
        ))}
      </div>

      {isOwner && (
        <form onSubmit={invite} className="mt-3 space-y-2">
          <div className="flex gap-2">
            <input
              className={inputCls}
              type="email"
              placeholder="teammate@company.com"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value)
                setError('')
              }}
            />
            <Button type="submit" size="sm" disabled={busy || !email.trim()} className="shrink-0">
              Invite
            </Button>
          </div>
          {link && (
            <div className="flex items-center gap-2 rounded-[6px] border border-white/8 bg-black/30 px-3 py-2">
              <span className="text-foreground/80 min-w-0 flex-1 truncate font-mono text-xs">{link}</span>
              <button
                type="button"
                onClick={() => navigator.clipboard?.writeText(link)}
                className="text-muted-foreground hover:text-foreground shrink-0 text-xs"
              >
                Copy
              </button>
              <button
                type="button"
                onClick={() => setLink('')}
                className="text-muted-foreground hover:text-foreground shrink-0"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          )}
        </form>
      )}
      {error && <p className="mt-2 text-xs text-rose-300">{error}</p>}
    </Section>
  )
}
