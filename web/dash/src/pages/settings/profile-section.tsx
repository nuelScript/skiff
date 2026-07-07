import { useState, type FormEvent } from 'react'
import { authService, type User } from '@/services/api.service'
import { errText } from '@/lib/errors'
import { Section, Field, SaveButton, inputCls } from './ui'

export function ProfileSection({ user, onSaved }: { user: User; onSaved: () => Promise<unknown> }) {
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
