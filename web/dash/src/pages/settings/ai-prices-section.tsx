import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { aiService, type AIPrice } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'
import { errText } from '@/lib/errors'
import { Button } from '@/components/ui/button'
import { Section, inputCls } from './ui'

const usd = (v: number) => '$' + v.toLocaleString(undefined, { maximumFractionDigits: 4 })

// AIPricesSection lets the team say what its models cost, so runs can show a cost. Skiff doesn't keep a price list: providers change theirs too often.
export function AIPricesSection({ isOwner }: { isOwner: boolean }) {
  const qc = useQueryClient()
  const { data } = useQuery({ queryKey: queryKeys.aiPrices, queryFn: () => aiService.prices() })
  const [model, setModel] = useState('')
  const [input, setInput] = useState('')
  const [output, setOutput] = useState('')
  const [error, setError] = useState('')
  const reload = () => qc.invalidateQueries({ queryKey: queryKeys.aiPrices })
  const save = useMutation({
    mutationFn: (p: AIPrice) => aiService.savePrice(p),
    onSuccess: () => {
      setModel('')
      setInput('')
      setOutput('')
      reload()
    },
    onError: (err) => setError(errText(err, 'Could not save the price.')),
  })
  const remove = useMutation({ mutationFn: (m: string) => aiService.removePrice(m), onSuccess: reload })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    setError('')
    const i = Number(input)
    const o = Number(output)
    if (!model.trim() || !Number.isFinite(i) || !Number.isFinite(o) || i < 0 || o < 0 || input === '' || output === '') {
      setError('Give the model and both prices, in dollars per million tokens.')
      return
    }
    save.mutate({ model: model.trim(), input: i, output: o })
  }

  const prices = data?.prices ?? []
  return (
    <Section
      title="Model prices"
      description="What your models cost, in dollars per million tokens, from your provider's pricing page. Agents' runs then show what they cost. A name also covers its dated versions, so gpt-4o-mini prices gpt-4o-mini-2024-07-18, but gpt-4 does not price gpt-4o."
    >
      {prices.length > 0 && (
        <ul className="mb-4 divide-y divide-foreground/6 overflow-hidden rounded-lg border border-foreground/8 text-sm">
          {prices.map((p) => (
            <li key={p.model} className="flex items-center justify-between gap-3 px-3 py-2">
              <span className="min-w-0 truncate font-mono">{p.model}</span>
              <span className="text-muted-foreground ml-auto shrink-0 text-xs">
                {usd(p.input)} in · {usd(p.output)} out
              </span>
              {isOwner && (
                <button
                  type="button"
                  aria-label={`Remove ${p.model}'s price`}
                  onClick={() => remove.mutate(p.model)}
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
        <form onSubmit={submit} className="grid gap-2 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_auto]">
          <input className={inputCls + ' font-mono'} aria-label="Model" placeholder="gpt-4o-mini" value={model} onChange={(e) => setModel(e.target.value)} />
          <input className={inputCls} aria-label="Input price per million tokens" inputMode="decimal" placeholder="$ in / 1M" value={input} onChange={(e) => setInput(e.target.value)} />
          <input className={inputCls} aria-label="Output price per million tokens" inputMode="decimal" placeholder="$ out / 1M" value={output} onChange={(e) => setOutput(e.target.value)} />
          <Button type="submit" size="sm" variant="outline" loading={save.isPending}>
            <Plus className="h-4 w-4" />
            Add
          </Button>
        </form>
      ) : (
        prices.length === 0 && <p className="text-muted-foreground text-xs">No prices yet. A team owner can add them.</p>
      )}
      {error && <p className="mt-2 text-xs text-danger">{error}</p>}
    </Section>
  )
}
