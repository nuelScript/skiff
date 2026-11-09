import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { queryKeys } from '@/constants/query-keys'
import { useCopy } from '@/hooks/use-copy'
import { errText } from '@/lib/errors'
import { relTime } from '@/lib/format'
import { useConfirm } from '@/providers/confirm-provider'
import { projectsService, type AccessToken } from '@/services/api.service'

// AccessTokensSection puts a key in front of the app, for MCP servers and APIs that clients call with a bearer token.
export function AccessTokensSection({ app, url }: { app: string; url?: string }) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const key = queryKeys.accessTokens(app)
  const { data } = useQuery({ queryKey: key, queryFn: () => projectsService.accessTokens(app) })
  const [name, setName] = useState('')
  const [created, setCreated] = useState<AccessToken | null>(null)
  const { copied, copy } = useCopy(1500)
  const tokens = data?.tokens ?? []
  // An app installed behind a key has no login of its own, so its last token stays.
  const keepLast = !!data?.required && tokens.length === 1

  const create = useMutation({
    mutationFn: () => projectsService.createAccessToken(app, name.trim()),
    onSuccess: (t) => {
      setCreated(t)
      setName('')
      qc.invalidateQueries({ queryKey: key })
    },
  })
  const revoke = useMutation({
    mutationFn: (id: string) => projectsService.revokeAccessToken(app, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: key }),
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (name.trim()) create.mutate()
  }

  const ask = async (t: AccessToken) => {
    const last = tokens.length === 1
    if (
      await confirm({
        title: `Revoke "${t.name}"?`,
        description: last
          ? `Clients using it stop getting through, and with no tokens left ${app} is open to anyone with its address again.`
          : 'Clients using it stop getting through straight away.',
        confirmText: 'Revoke',
        destructive: true,
      })
    ) {
      if (created?.id === t.id) setCreated(null)
      revoke.mutate(t.id)
    }
  }

  const error = create.error ?? revoke.error
  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-semibold">Access tokens</h2>
        <p className="text-muted-foreground mt-1 text-xs">
          Once the app has a token, every request to it must carry one as{' '}
          <code className="font-mono">Authorization: Bearer …</code>, and Skiff removes it before the request reaches the
          app. For MCP servers and APIs called by other programs.{' '}
          {keepLast ? (
            <span className="text-foreground">
              On, and required: {app} has no login of its own, so it always keeps a token. To change it, add a new one,
              then revoke this one.
            </span>
          ) : tokens.length > 0 ? (
            <span className="text-foreground">On: requests without a token are refused.</span>
          ) : (
            'Off: anyone with the address gets through.'
          )}
        </p>
      </div>

      {created?.token && (
        <div className="rounded-lg border border-success/25 bg-success/5 p-3">
          <p className="mb-2 text-xs text-success/90">Copy this token now; it won't be shown again.</p>
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded bg-inset px-2.5 py-1.5 font-mono text-xs">{created.token}</code>
            <Button type="button" size="sm" variant="outline" onClick={() => created.token && copy(created.token)}>
              {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
              {copied ? 'Copied' : 'Copy'}
            </Button>
          </div>
          {url && (
            <pre className="text-muted-foreground mt-2 overflow-x-auto font-mono text-[11px]">
              {`claude mcp add --transport http ${app} ${url}/mcp \\\n  --header "Authorization: Bearer ${created.token}"`}
            </pre>
          )}
        </div>
      )}

      {tokens.length > 0 && (
        <ul className="divide-y divide-foreground/6 overflow-hidden rounded-lg border border-foreground/8">
          {tokens.map((t) => (
            <li key={t.id} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
              <div className="min-w-0">
                <p className="truncate font-medium">{t.name}</p>
                <p className="text-muted-foreground text-xs">created {relTime(t.created)}</p>
              </div>
              <button
                type="button"
                onClick={() => void ask(t)}
                disabled={keepLast}
                aria-label={`Revoke ${t.name}`}
                title={keepLast ? `${app} has no login of its own. Add another token before revoking this one.` : undefined}
                className="text-muted-foreground cursor-pointer p-1 transition-colors hover:text-danger disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:text-muted-foreground"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </li>
          ))}
        </ul>
      )}

      <form onSubmit={submit} className="flex items-start gap-2">
        <Input
          aria-label="Token name"
          className="sm:max-w-64"
          placeholder="Token name, e.g. claude-desktop"
          value={name}
          onChange={(e) => {
            setName(e.target.value)
            create.reset()
          }}
        />
        <Button size="sm" type="submit" variant="outline" loading={create.isPending} disabled={!name.trim()}>
          <Plus className="h-4 w-4" />
          {tokens.length === 0 ? 'Require a token' : 'Add token'}
        </Button>
      </form>
      {error && <p className="text-xs text-danger">{errText(error, 'Something went wrong.')}</p>}
    </section>
  )
}
