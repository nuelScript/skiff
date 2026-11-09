import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, Plus, Trash2 } from 'lucide-react'
import { relTime } from '@/lib/format'
import {
  projectsService,
  tokensService,
  SCOPE_WORDS,
  TOKEN_SCOPES,
  type ApiToken,
  type TokenScope,
} from '@/services/api.service'
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
  const [created, setCreated] = useState<ApiToken | null>(null)
  const [limiting, setLimiting] = useState(false)
  const [scopes, setScopes] = useState<TokenScope[]>(TOKEN_SCOPES.map((s) => s.value))
  const [apps, setApps] = useState<string[]>([])
  const { copied, copy } = useCopy(1500)
  const { data: projects = [] } = useQuery({
    queryKey: queryKeys.apps,
    queryFn: () => projectsService.list(),
    enabled: limiting,
  })

  const toggle = <T,>(list: T[], v: T) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v])

  const create = useMutation({
    mutationFn: () => tokensService.create(name.trim(), limiting ? scopes : [], limiting ? apps : []),
    onSuccess: (t) => {
      setCreated(t)
      setName('')
      setLimiting(false)
      setScopes(TOKEN_SCOPES.map((s) => s.value))
      setApps([])
      qc.invalidateQueries({ queryKey: queryKeys.tokens })
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (name.trim() && (!limiting || scopes.length > 0)) create.mutate()
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
        <div className="mb-4 rounded-lg border border-success/25 bg-success/5 p-3">
          <p className="mb-2 text-xs text-success/90">
            Copy your token now — it won't be shown again.
          </p>
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded bg-inset px-2.5 py-1.5 font-mono text-xs">
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

      <form onSubmit={submit} className="flex gap-2">
        <input
          className={inputCls}
          placeholder="Token name (e.g. ci-deploy)"
          value={name}
          onChange={(e) => {
            setName(e.target.value)
            create.reset()
          }}
        />
        <Button type="submit" size="sm" loading={create.isPending} disabled={!name.trim() || (limiting && scopes.length === 0)}>
          <Plus className="h-4 w-4" />
          Create
        </Button>
      </form>
      {!limiting ? (
        <button
          type="button"
          onClick={() => setLimiting(true)}
          className="text-muted-foreground hover:text-foreground mt-2 cursor-pointer text-xs underline underline-offset-2"
        >
          Limit what it can do, or to some projects
        </button>
      ) : (
        <div className="mt-3 grid gap-4 rounded-lg border border-foreground/8 p-3 sm:grid-cols-2">
          <fieldset className="grid gap-1.5">
            <legend className="mb-1 text-xs font-medium">It can</legend>
            {TOKEN_SCOPES.map((s) => (
              <label key={s.value} className="flex cursor-pointer items-center gap-2 text-xs">
                <input type="checkbox" checked={scopes.includes(s.value)} onChange={() => setScopes(toggle(scopes, s.value))} />
                {s.label}
              </label>
            ))}
            {scopes.length === 0 && <p className="text-xs text-danger">Pick at least one.</p>}
          </fieldset>
          <fieldset className="grid content-start gap-1.5">
            <legend className="mb-1 text-xs font-medium">On</legend>
            <label className="flex cursor-pointer items-center gap-2 text-xs">
              <input type="checkbox" checked={apps.length === 0} onChange={() => setApps([])} />
              Every project, including new ones
            </label>
            {projects.map((p) => (
              <label key={p.name} className="flex cursor-pointer items-center gap-2 text-xs">
                <input type="checkbox" checked={apps.includes(p.name)} onChange={() => setApps(toggle(apps, p.name))} />
                <span className="truncate font-mono">{p.name}</span>
              </label>
            ))}
          </fieldset>
        </div>
      )}
      {create.isError && (
        <p className="mt-2 text-xs text-danger">
          {errText(create.error, 'Could not create token.')}
        </p>
      )}

      {tokens.length > 0 && (
        <ul className="mt-4 divide-y divide-foreground/6 overflow-hidden rounded-lg border border-foreground/8">
          {tokens.map((t) => (
            <li key={t.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{t.name}</p>
                <p className="text-muted-foreground text-xs">
                  created {relTime(t.created)} ·{' '}
                  {t.lastUsed ? `last used ${relTime(t.lastUsed)}` : 'never used'}
                </p>
                {!!(t.scopes?.length || t.apps?.length) && (
                  <p className="text-muted-foreground mt-0.5 text-xs">
                    {t.scopes?.length ? `can use ${t.scopes.map((s) => SCOPE_WORDS[s]).join(', ')}` : 'can do everything'}
                    {t.apps?.length ? ` · only ${t.apps.join(', ')}` : ''}
                  </p>
                )}
              </div>
              <button
                type="button"
                onClick={() => revoke(t)}
                aria-label={`Revoke ${t.name}`}
                className="text-muted-foreground p-1 transition-colors hover:text-danger"
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
