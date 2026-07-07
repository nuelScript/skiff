import { useState, type FormEvent } from 'react'
import { Check } from 'lucide-react'
import { authService } from '@/services/api.service'
import { errText } from '@/lib/errors'
import { Button } from '@/components/ui/button'
import { Section, RevealInput } from './ui'

export function PasswordSection() {
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
