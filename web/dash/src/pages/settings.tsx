import { useState, type ComponentProps, type FormEvent, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Eye, EyeOff, GitBranch, Trash2, X } from 'lucide-react'
import { useAuthContext } from '@/context/auth-context'
import {
  authService,
  githubService,
  type Member,
  type GithubStatus,
  type Team,
  type User,
} from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'
import { Button } from '@/components/ui/button'
import { errText } from '@/lib/errors'

const inputCls =
  'h-9 w-full rounded-[6px] border border-white/12 bg-black/30 px-3 text-sm outline-none placeholder:text-white/25 focus-visible:border-white/30 disabled:opacity-50'

function RevealInput(props: ComponentProps<'input'>) {
  const [show, setShow] = useState(false)
  return (
    <div className="relative">
      <input {...props} type={show ? 'text' : 'password'} className={inputCls + ' pr-9'} />
      <button
        type="button"
        tabIndex={-1}
        aria-label={show ? 'Hide password' : 'Show password'}
        onClick={() => setShow((s) => !s)}
        className="text-muted-foreground hover:text-foreground absolute inset-y-0 right-0 flex items-center px-2.5 transition-colors"
      >
        {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  )
}

export default function SettingsPage() {
  const { me, refresh } = useAuthContext()
  const qc = useQueryClient()
  const isOwner = me?.role === 'owner'
  const team = me?.teams?.find((t) => t.id === me?.team)

  const { data: members = [] } = useQuery<Member[]>({
    queryKey: queryKeys.members,
    queryFn: () => authService.members(),
  })
  const { data: gh } = useQuery<GithubStatus>({
    queryKey: queryKeys.github.status,
    queryFn: () => githubService.status(),
  })
  const reloadMembers = () => qc.invalidateQueries({ queryKey: queryKeys.members })

  return (
    <div className="px-8 py-8">
      <header className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight">Settings</h1>
        <p className="text-muted-foreground mt-1 text-sm">Your account, team, and connections.</p>
      </header>

      <div className="max-w-3xl space-y-4">
        {me?.user && <ProfileSection user={me.user} onSaved={refresh} />}
        <PasswordSection />
        <TeamSection team={team} isOwner={isOwner} onSaved={refresh} />
        <MembersSection
          members={members}
          meId={me?.user?.id ?? ''}
          isOwner={isOwner}
          onChange={reloadMembers}
        />
        <ConnectionsSection gh={gh} />
      </div>
    </div>
  )
}

function Section({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children: ReactNode
}) {
  return (
    <section className="rounded-xl border border-white/8 bg-linear-to-b from-white/2 to-transparent p-5">
      <div className="mb-4">
        <h2 className="text-sm font-semibold">{title}</h2>
        {description && <p className="text-muted-foreground mt-1 text-xs">{description}</p>}
      </div>
      {children}
    </section>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="grid gap-1.5 sm:grid-cols-[7rem_1fr] sm:items-center sm:gap-4">
      <span className="text-muted-foreground text-xs">{label}</span>
      <div>{children}</div>
    </label>
  )
}

function SaveButton({ busy, saved, disabled }: { busy: boolean; saved: boolean; disabled: boolean }) {
  return (
    <Button type="submit" size="sm" disabled={disabled || busy}>
      {saved ? (
        <>
          <Check className="h-4 w-4" />
          Saved
        </>
      ) : busy ? (
        'Saving…'
      ) : (
        'Save'
      )}
    </Button>
  )
}

function ProfileSection({ user, onSaved }: { user: User; onSaved: () => Promise<unknown> }) {
  const [name, setName] = useState(user.name)
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')
  const dirty = name.trim() !== '' && name.trim() !== user.name

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await authService.updateProfile(name.trim())
      await onSaved()
      setSaved(true)
      setTimeout(() => setSaved(false), 1500)
    } catch (err) {
      setError(errText(err, 'Could not save your profile.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Section title="Profile" description="Your name as it appears across the dashboard.">
      <form onSubmit={submit} className="space-y-3">
        <Field label="Name">
          <input className={inputCls} value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Email">
          <input className={inputCls} value={user.email} disabled readOnly />
        </Field>
        {error && <p className="text-xs text-rose-300">{error}</p>}
        <div className="flex justify-end">
          <SaveButton busy={busy} saved={saved} disabled={!dirty} />
        </div>
      </form>
    </Section>
  )
}

function PasswordSection() {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await authService.changePassword(current, next)
      setCurrent('')
      setNext('')
      setSaved(true)
      setTimeout(() => setSaved(false), 1500)
    } catch (err) {
      setError(errText(err, 'Could not change your password.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Section title="Password" description="Use at least 8 characters.">
      <form onSubmit={submit} className="space-y-3">
        <RevealInput
          placeholder="Current password"
          autoComplete="current-password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
        />
        <RevealInput
          placeholder="New password"
          autoComplete="new-password"
          value={next}
          onChange={(e) => setNext(e.target.value)}
        />
        {error && <p className="text-xs text-rose-300">{error}</p>}
        <div className="flex justify-end">
          <Button type="submit" size="sm" disabled={busy || !current || next.length < 8}>
            {saved ? (
              <>
                <Check className="h-4 w-4" />
                Updated
              </>
            ) : busy ? (
              'Updating…'
            ) : (
              'Update password'
            )}
          </Button>
        </div>
      </form>
    </Section>
  )
}

function TeamSection({
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
    if (!confirm(`${verb} ${team?.name ?? 'this team'}? This can't be undone.`)) return
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

function MembersSection({
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
    if (!confirm(`Remove ${m.user.name} from the team?`)) return
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

function ConnectionsSection({ gh }: { gh?: GithubStatus }) {
  const connected = gh?.installed
  return (
    <Section title="Connections" description="Source providers Skiff deploys from.">
      <div className="flex items-center gap-3 rounded-lg border border-white/8 px-3.5 py-3">
        <span className="grid h-9 w-9 shrink-0 place-items-center rounded-md border border-white/10 bg-linear-to-br from-white/[0.07] to-transparent">
          <GitBranch className="h-4 w-4 text-white/70" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">GitHub</p>
          <p className="text-muted-foreground truncate text-xs">
            {connected
              ? 'Connected' + (gh?.slug ? ` · ${gh.slug}` : '')
              : 'Not connected — deploy public repos, or connect for private ones.'}
          </p>
        </div>
        {connected ? (
          <span className="flex shrink-0 items-center gap-1.5 rounded-full border border-emerald-400/20 bg-emerald-400/10 px-2.5 py-1 text-[11px] font-medium text-emerald-300">
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
            Connected
          </span>
        ) : (
          <a
            href="/server"
            className="text-muted-foreground hover:border-white/25 hover:text-foreground shrink-0 rounded-[6px] border border-white/12 px-2.5 py-1 text-xs transition"
          >
            Set up
          </a>
        )}
      </div>
    </Section>
  )
}
