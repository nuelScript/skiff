import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, Plus, Trash2 } from 'lucide-react'
import { relTime } from '@/lib/format'
import { tokensService, type ApiToken } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { Button } from '@/components/ui/button'
import { useConfirm } from '@/providers/confirm-provider'
import { useCopy } from '@/hooks/use-copy'
import { Section, inputCls } from './ui'

export function TokensSection() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const { data: tokens = [] } = useQuery<ApiToken[]>({
    queryKey: queryKeys.tokens,
    queryFn: () => tokensService.list(),
  })
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [created, setCreated] = useState<ApiToken | null>(null)
  const { copied, copy } = useCopy(1500)
  const [error, setError] = useState('')

  const create = async (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setError('')
    setBusy(true)
    try {
      const t = await tokensService.create(name.trim())
      setCreated(t)
      setName('')
      await qc.invalidateQueries({ queryKey: queryKeys.tokens })
    } catch (err) {
      setError(errText(err, 'Could not create token.'))
    } finally {
      setBusy(false)
    }
  }

  const revoke = async (t: ApiToken) => {
    if (
      !(await confirm({
        title: `Revoke "${t.name}"?`,
        description: 'Any CI using this token will stop working immediately.',
        confirmText: 'Revoke',
        destructive: true,
      }))
    )
      return
    if (created?.id === t.id) setCreated(null)
    await tokensService.revoke(t.id)
    qc.invalidateQueries({ queryKey: queryKeys.tokens })
  }


  return (
    <Section
      title="API tokens"
      description="Deploy and manage apps from CI over the token-authenticated /api/v1 API."
    >
      {created?.token && (
        <div className="mb-4 rounded-lg border border-emerald-400/25 bg-emerald-400/5 p-3">
          <p className="mb-2 text-xs text-emerald-200/90">
            Copy your token now — it won't be shown again.
          </p>
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded bg-black/40 px-2.5 py-1.5 font-mono text-xs">
              {created.token}
            </code>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => created?.token && copy(created.token)}
            >
              {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
              {copied ? 'Copied' : 'Copy'}
            </Button>
          </div>
        </div>
      )}

      <form onSubmit={create} className="flex gap-2">
        <input
          className={inputCls}
          placeholder="Token name (e.g. ci-deploy)"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <Button type="submit" size="sm" disabled={busy || !name.trim()}>
          <Plus className="h-4 w-4" />
          Create
        </Button>
      </form>
      {error && <p className="mt-2 text-xs text-rose-300">{error}</p>}

      {tokens.length > 0 && (
        <ul className="mt-4 divide-y divide-white/6 overflow-hidden rounded-lg border border-white/8">
          {tokens.map((t) => (
            <li key={t.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{t.name}</p>
                <p className="text-muted-foreground text-xs">
                  created {relTime(t.created)} · {t.lastUsed ? `last used ${relTime(t.lastUsed)}` : 'never used'}
                </p>
              </div>
              <button
                type="button"
                onClick={() => revoke(t)}
                aria-label={`Revoke ${t.name}`}
                className="text-muted-foreground p-1 transition-colors hover:text-rose-300"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </li>
          ))}
        </ul>
      )}

      <p className="text-muted-foreground mt-4 overflow-x-auto font-mono text-[11px] whitespace-nowrap">
        curl -H &quot;Authorization: Bearer &lt;token&gt;&quot; {window.location.origin}/api/v1/apps
      </p>
    </Section>
  )
}
