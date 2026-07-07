import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell } from 'lucide-react'
import { alertsService } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { Button } from '@/components/ui/button'
import { Section, Field, SaveButton, inputCls } from './ui'

export function AlertsSection() {
  const qc = useQueryClient()
  const { data } = useQuery({ queryKey: queryKeys.alerts, queryFn: () => alertsService.config() })
  const [email, setEmail] = useState('')
  const [slackUrl, setSlackUrl] = useState('')
  const [webhookUrl, setWebhookUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')
  const [testing, setTesting] = useState(false)
  const [testMsg, setTestMsg] = useState('')
  const hydrated = useRef(false)

  useEffect(() => {
    if (data && !hydrated.current) {
      setEmail(data.email)
      setSlackUrl(data.slackUrl)
      setWebhookUrl(data.webhookUrl)
      hydrated.current = true
    }
  }, [data])

  const dirty =
    !!data &&
    (email.trim() !== data.email ||
      slackUrl.trim() !== data.slackUrl ||
      webhookUrl.trim() !== data.webhookUrl)
  const hasChannels = !!data && !!(data.email || data.slackUrl || data.webhookUrl)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setTestMsg('')
    setBusy(true)
    try {
      await alertsService.save({
        email: email.trim(),
        slackUrl: slackUrl.trim(),
        webhookUrl: webhookUrl.trim(),
      })
      await qc.invalidateQueries({ queryKey: queryKeys.alerts })
      setSaved(true)
      setTimeout(() => setSaved(false), 1500)
    } catch (err) {
      setError(errText(err, 'Could not save alerts.'))
    } finally {
      setBusy(false)
    }
  }

  const runTest = async () => {
    setTesting(true)
    setTestMsg('')
    try {
      const { results } = await alertsService.test()
      setTestMsg(
        results.length === 0
          ? 'No channels configured.'
          : results.map((r) => `${r.channel} ${r.ok ? '✓' : '✗'}`).join('  ·  '),
      )
    } catch {
      setTestMsg('Test failed.')
    } finally {
      setTesting(false)
    }
  }

  return (
    <Section
      title="Alerts"
      description="Get notified when a deploy fails, an app goes down, or 5xx errors spike."
    >
      <form onSubmit={submit} className="space-y-3">
        <Field label="Email">
          <input
            className={inputCls}
            type="email"
            placeholder="you@example.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </Field>
        <Field label="Slack">
          <input
            className={inputCls}
            placeholder="https://hooks.slack.com/services/…"
            value={slackUrl}
            onChange={(e) => setSlackUrl(e.target.value)}
          />
        </Field>
        <Field label="Webhook">
          <input
            className={inputCls}
            placeholder="https://your-service/hook"
            value={webhookUrl}
            onChange={(e) => setWebhookUrl(e.target.value)}
          />
        </Field>
        {email.trim() !== '' && data && !data.smtp && (
          <p className="text-muted-foreground text-xs sm:pl-32">
            Email needs SMTP configured on the server (
            <span className="font-mono">SKIFF_SMTP_*</span>).
          </p>
        )}
        {error && <p className="text-xs text-rose-300">{error}</p>}
        <div className="flex items-center justify-end gap-3">
          {testMsg && <span className="text-muted-foreground font-mono text-xs">{testMsg}</span>}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={runTest}
            disabled={!hasChannels || dirty || testing}
          >
            <Bell className="h-4 w-4" />
            {testing ? 'Sending…' : 'Send test'}
          </Button>
          <SaveButton busy={busy} saved={saved} disabled={!dirty} />
        </div>
      </form>
    </Section>
  )
}
