import { useState, type FormEvent } from 'react'
import { authService, type Team } from '@/services/api.service'
import { errText } from '@/lib/errors'
import { useConfirm } from '@/providers/confirm-provider'
import { Section, Field, SaveButton, inputCls } from './ui'

export function TeamSection({
  team,
  isOwner,
  onSaved,
}: {
  team?: Team
  isOwner: boolean
  onSaved: () => Promise<unknown>
}) {
  const [name, setName] = useState(team?.name ?? '')
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')
  const [dangerBusy, setDangerBusy] = useState(false)
  const confirm = useConfirm()
  const dirty = name.trim() !== '' && name.trim() !== (team?.name ?? '')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await authService.renameTeam(name.trim())
      await onSaved()
      setSaved(true)
      setTimeout(() => setSaved(false), 1500)
    } catch (err) {
      setError(errText(err, 'Could not rename the team.'))
    } finally {
      setBusy(false)
    }
  }

  const leaveOrDelete = async () => {
    const verb = isOwner ? 'Delete' : 'Leave'
    if (
      !(await confirm({
        title: `${verb} ${team?.name ?? 'this team'}?`,
        description: "This can't be undone.",
        confirmText: verb,
        destructive: true,
      }))
    )
      return
    setError('')
    setDangerBusy(true)
    try {
      await (isOwner ? authService.deleteTeam() : authService.leaveTeam())
      window.location.href = '/'
    } catch (err) {
      setError(errText(err, `Could not ${verb.toLowerCase()} the team.`))
      setDangerBusy(false)
    }
  }

  return (
    <Section title="Team" description="Projects, databases, and domains are scoped to a team.">
      <form onSubmit={submit} className="space-y-3">
        <Field label="Name">
          <input
            className={inputCls}
            value={name}
            disabled={!isOwner}
            onChange={(e) => setName(e.target.value)}
          />
        </Field>
        <div className="flex items-center justify-between gap-3">
          <span className="text-muted-foreground text-xs">
            Slug <span className="text-foreground/70 font-mono">{team?.slug}</span>
          </span>
          {isOwner ? (
            <SaveButton busy={busy} saved={saved} disabled={!dirty} />
          ) : (
            <span className="text-muted-foreground text-xs">Only owners can rename the team.</span>
          )}
        </div>
      </form>

      <div className="mt-4 flex items-center justify-between gap-3 border-t border-white/5 pt-4">
        <p className="text-muted-foreground text-[11px]">
          {isOwner
            ? 'Deleting removes the team for everyone. Remove its projects and databases first.'
            : "You'll lose access to this team's projects."}
        </p>
        <button
          type="button"
          onClick={leaveOrDelete}
          disabled={dangerBusy}
          className="shrink-0 rounded-[6px] border border-rose-500/30 px-2.5 py-1 text-xs text-rose-300 transition hover:bg-rose-500/10 disabled:opacity-50"
        >
          {isOwner ? 'Delete team' : 'Leave team'}
        </button>
      </div>
      {error && <p className="mt-2 text-xs text-rose-300">{error}</p>}
    </Section>
  )
}
