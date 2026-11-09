import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { aiService, type AIProvider, type AIProviderKind } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { relTime } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { useConfirm } from '@/providers/confirm-provider'
import { aiProviderSchema, type AIProviderInput } from '@/validations/ai-provider'
import { Section, Field, RevealInput, inputCls } from './ui'

const kinds: { kind: AIProviderKind; label: string; name: string; hint: string }[] = [
  { kind: 'openai', label: 'OpenAI', name: 'openai', hint: 'sk-…' },
  { kind: 'anthropic', label: 'Anthropic', name: 'anthropic', hint: 'sk-ant-…' },
  { kind: 'custom', label: 'Other', name: '', hint: 'Your API key' },
]

const kindLabel = (k: AIProviderKind) => kinds.find((x) => x.kind === k)?.label ?? k

export function AIProvidersSection({ isOwner }: { isOwner: boolean }) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const { data } = useQuery({ queryKey: queryKeys.aiProviders, queryFn: () => aiService.providers() })
  const form = useForm<AIProviderInput>({
    resolver: zodResolver(aiProviderSchema),
    defaultValues: { kind: 'openai', name: 'openai', baseUrl: '', key: '' },
  })
  const { errors } = form.formState
  const kind = form.watch('kind')
  const reload = () => qc.invalidateQueries({ queryKey: queryKeys.aiProviders })

  const save = useMutation({
    mutationFn: (v: AIProviderInput) => aiService.saveProvider(v),
    onSuccess: () => {
      form.reset({ kind: 'openai', name: 'openai', baseUrl: '', key: '' })
      reload()
    },
  })
  const remove = useMutation({ mutationFn: (name: string) => aiService.removeProvider(name), onSuccess: reload })

  const ask = async (p: AIProvider) => {
    if (
      await confirm({
        title: `Remove ${p.name}?`,
        description: `Apps calling ${p.name} through Skiff stop getting answers. The key is deleted from Skiff, not from ${kindLabel(p.kind)}.`,
        confirmText: 'Remove',
        destructive: true,
      })
    )
      remove.mutate(p.name)
  }

  const providers = data?.providers ?? []
  return (
    <Section
      title="AI providers"
      description="Keys for the model APIs your apps use. Skiff keeps them: an app with AI access calls the model through Skiff with its own token, never sees the key, and stops at the budget you set for it."
    >
      {providers.length > 0 && (
        <ul className="mb-4 divide-y divide-foreground/6 overflow-hidden rounded-lg border border-foreground/8">
          {providers.map((p) => (
            <li key={p.name} className="flex items-center justify-between gap-3 px-3 py-2.5">
              <div className="min-w-0">
                <p className="truncate font-mono text-sm">{p.name}</p>
                <p className="text-muted-foreground truncate text-xs">
                  {kindLabel(p.kind)}
                  {p.baseUrl && ` · ${p.baseUrl}`}
                  {p.keyHint && ` · key ${p.keyHint}`} · added {relTime(p.created)}
                </p>
              </div>
              {isOwner && (
                <button
                  type="button"
                  onClick={() => void ask(p)}
                  aria-label={`Remove ${p.name}`}
                  className="text-muted-foreground cursor-pointer p-1 transition-colors hover:text-danger"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}

      {isOwner ? (
        <form onSubmit={form.handleSubmit((v) => save.mutate(v))} className="space-y-3">
          <div role="radiogroup" aria-label="Provider" className="flex flex-wrap gap-1.5">
            {kinds.map((k) => (
              <button
                key={k.kind}
                type="button"
                role="radio"
                aria-checked={kind === k.kind}
                onClick={() => {
                  form.setValue('kind', k.kind)
                  form.setValue('name', k.name)
                  save.reset()
                }}
                className={cn(
                  'cursor-pointer rounded-lg border px-3 py-1.5 text-xs transition-colors',
                  kind === k.kind
                    ? 'border-foreground/30 bg-foreground/5 font-medium'
                    : 'text-muted-foreground border-foreground/8 hover:text-foreground',
                )}
              >
                {k.label}
              </button>
            ))}
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Name">
              <input
                className={inputCls + ' font-mono'}
                placeholder="openrouter"
                aria-invalid={!!errors.name}
                {...form.register('name')}
              />
              {errors.name && <p className="mt-1 text-xs text-danger">{errors.name.message}</p>}
            </Field>
            <Field label="API key">
              <RevealInput
                placeholder={kinds.find((k) => k.kind === kind)?.hint}
                autoComplete="new-password"
                aria-invalid={!!errors.key}
                {...form.register('key')}
              />
              {errors.key && <p className="mt-1 text-xs text-danger">{errors.key.message}</p>}
            </Field>
          </div>
          {kind === 'custom' && (
            <Field label="API address">
              <input
                className={inputCls + ' font-mono'}
                placeholder="https://openrouter.ai/api"
                aria-invalid={!!errors.baseUrl}
                {...form.register('baseUrl')}
              />
              {errors.baseUrl ? (
                <p className="mt-1 text-xs text-danger">{errors.baseUrl.message}</p>
              ) : (
                <p className="text-muted-foreground mt-1 text-xs">
                  Any API that works like OpenAI's: OpenRouter, Groq, Together, or a model running on your own server.
                </p>
              )}
            </Field>
          )}
          {save.isError && <p className="text-xs text-danger">{errText(save.error, 'Could not save the provider.')}</p>}
          <div className="flex justify-end">
            <Button type="submit" size="sm" loading={save.isPending}>
              <Plus className="h-4 w-4" />
              Add provider
            </Button>
          </div>
        </form>
      ) : (
        providers.length === 0 && (
          <p className="text-muted-foreground text-xs">No providers yet. A team owner can add one.</p>
        )
      )}
      {data && !data.gateway && (
        <p className="mt-3 text-xs text-warning">
          Skiff couldn't find Docker's network on this server, so apps can't reach the AI gateway yet.
        </p>
      )}
    </Section>
  )
}
