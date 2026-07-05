import { useMemo, useState, type FormEvent } from 'react'
import { Globe, Plus, Trash2, ChevronDown, ExternalLink, Search, ShieldCheck, X } from 'lucide-react'
import { useApps } from '@/hooks/use-apps'
import { useDomains } from '@/hooks/use-domains'
import { Button } from '@/components/ui/button'
import type { App, Domain } from '@/services/api.service'

// An app's <app>.<domain> host (the CNAME target), derived from its URL.
const targetHost = (apps: App[], app: string): string => {
  const a = apps.find((x) => x.name === app)
  return a ? a.url.replace(/^https?:\/\//, '') : app
}

export default function DomainsPage() {
  const { apps } = useApps()
  const { domains, add, remove } = useDomains()

  const [query, setQuery] = useState('')
  const [adding, setAdding] = useState(false)
  const [app, setApp] = useState('')
  const [host, setHost] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const chosenApp = app || apps[0]?.name || ''

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return q ? domains.filter((d) => d.host.includes(q) || d.app.toLowerCase().includes(q)) : domains
  }, [domains, query])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (!host.trim() || !chosenApp) return
    setBusy(true)
    try {
      await add(chosenApp, host.trim())
      setHost('')
      setAdding(false)
    } catch (err: unknown) {
      const r = (err as { response?: { data?: string } })?.response?.data
      setError(typeof r === 'string' && r ? r.trim() : 'Could not add that domain.')
    } finally {
      setBusy(false)
    }
  }

  const active = domains.filter((d) => d.pointsHere).length

  return (
    <div className="mx-auto max-w-4xl px-8 py-8">
      <header className="mb-6 flex items-end justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Domains</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Serve any app on your own domain, with an automatic HTTPS certificate.
          </p>
        </div>
        {domains.length > 0 && !adding && (
          <Button size="sm" onClick={() => setAdding(true)} className="shrink-0">
            <Plus className="h-4 w-4" />
            Add domain
          </Button>
        )}
      </header>

      {/* add composer */}
      {(adding || domains.length === 0) && (
        <AddComposer
          apps={apps}
          chosenApp={chosenApp}
          onApp={setApp}
          host={host}
          onHost={(v) => {
            setHost(v)
            setError('')
          }}
          onSubmit={submit}
          onCancel={
            domains.length > 0
              ? () => {
                  setAdding(false)
                  setHost('')
                  setError('')
                }
              : undefined
          }
          busy={busy}
          error={error}
        />
      )}

      {domains.length > 0 && (
        <>
          {/* toolbar */}
          <div className="mt-6 mb-3 flex items-center justify-between gap-3">
            <div className="relative max-w-xs flex-1">
              <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2" />
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search domains…"
                className="h-9 w-full rounded-[6px] border border-white/12 bg-white/2 pr-3 pl-8.5 text-sm outline-none placeholder:text-white/30 focus-visible:border-white/25"
              />
            </div>
            <span className="text-muted-foreground shrink-0 font-mono text-[11px] tabular-nums">
              {active}/{domains.length} active
            </span>
          </div>

          {/* list */}
          <div className="overflow-hidden rounded-xl border border-white/8">
            {filtered.length === 0 ? (
              <p className="text-muted-foreground p-8 text-center text-sm">No domains match “{query}”.</p>
            ) : (
              filtered.map((d, i) => (
                <DomainRow
                  key={d.host}
                  domain={d}
                  target={targetHost(apps, d.app)}
                  onRemove={() => {
                    if (confirm(`Remove ${d.host}?`)) remove(d.host)
                  }}
                  delay={i * 40}
                />
              ))
            )}
          </div>
        </>
      )}
    </div>
  )
}

function AddComposer({
  apps,
  chosenApp,
  onApp,
  host,
  onHost,
  onSubmit,
  onCancel,
  busy,
  error,
}: {
  apps: App[]
  chosenApp: string
  onApp: (v: string) => void
  host: string
  onHost: (v: string) => void
  onSubmit: (e: FormEvent) => void
  onCancel?: () => void
  busy: boolean
  error: string
}) {
  return (
    <form
      onSubmit={onSubmit}
      className="animate-rise rounded-xl border border-white/10 bg-linear-to-b from-white/3 to-transparent p-4"
    >
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-medium">Add a domain</h2>
        {onCancel && (
          <button
            type="button"
            onClick={onCancel}
            className="text-muted-foreground hover:text-foreground -mr-1 -mt-1 p-1"
          >
            <X className="h-4 w-4" />
          </button>
        )}
      </div>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <input
          value={host}
          onChange={(e) => onHost(e.target.value)}
          autoFocus
          placeholder="www.yourdomain.com"
          className="h-9 min-w-0 flex-1 rounded-[6px] border border-white/12 bg-black/30 px-3 font-mono text-sm outline-none placeholder:text-white/25 focus-visible:border-white/30"
        />
        <span className="text-muted-foreground hidden shrink-0 text-xs sm:block">→</span>
        <div className="relative shrink-0 sm:w-44">
          <select
            value={chosenApp}
            onChange={(e) => onApp(e.target.value)}
            className="h-9 w-full cursor-pointer appearance-none rounded-[6px] border border-white/12 bg-black/30 pr-8 pl-3 text-sm outline-none focus-visible:border-white/30"
          >
            {apps.length === 0 && <option value="">No apps yet</option>}
            {apps.map((a) => (
              <option key={a.name} value={a.name} className="bg-neutral-900">
                {a.name}
              </option>
            ))}
          </select>
          <ChevronDown className="text-muted-foreground pointer-events-none absolute top-1/2 right-2.5 h-4 w-4 -translate-y-1/2" />
        </div>
        <Button type="submit" size="sm" disabled={busy || !host.trim() || !chosenApp} className="shrink-0">
          Add
        </Button>
      </div>
      {error ? (
        <p className="mt-2 text-xs text-rose-300">{error}</p>
      ) : (
        <p className="text-muted-foreground mt-2 text-xs">
          You'll point DNS at your server; the HTTPS certificate is issued automatically.
        </p>
      )}
    </form>
  )
}

function DomainRow({
  domain,
  target,
  onRemove,
  delay,
}: {
  domain: Domain
  target: string
  onRemove: () => void
  delay: number
}) {
  return (
    <div
      className="animate-rise group border-b border-white/5 px-4 py-3.5 transition-colors last:border-0 hover:bg-white/2"
      style={{ animationDelay: delay + 'ms' }}
    >
      <div className="flex items-center gap-3.5">
        {/* glyph */}
        <span className="grid h-9 w-9 shrink-0 place-items-center rounded-md border border-white/10 bg-linear-to-br from-white/[0.07] to-transparent">
          <Globe className="h-4 w-4 text-white/55" />
        </span>

        {/* name + connection */}
        <div className="min-w-0 flex-1">
          <a
            href={'https://' + domain.host}
            target="_blank"
            rel="noreferrer"
            className="hover:text-foreground group/link flex items-center gap-1.5 text-sm font-medium"
          >
            <span className="truncate font-mono">{domain.host}</span>
            <ExternalLink className="h-3 w-3 shrink-0 opacity-0 transition group-hover/link:opacity-60" />
          </a>
          <p className="text-muted-foreground mt-0.5 truncate text-xs">
            Connected to <span className="text-foreground/70">{domain.app}</span>
          </p>
        </div>

        {/* status */}
        {domain.pointsHere ? (
          <span className="flex shrink-0 items-center gap-1.5 rounded-full border border-emerald-400/20 bg-emerald-400/10 px-2.5 py-1 text-[11px] font-medium text-emerald-300">
            <ShieldCheck className="h-3.5 w-3.5" />
            Active
          </span>
        ) : (
          <span className="flex shrink-0 items-center gap-1.5 rounded-full border border-amber-400/20 bg-amber-400/10 px-2.5 py-1 text-[11px] font-medium text-amber-300">
            <span className="pulse-dot h-1.5 w-1.5 rounded-full bg-amber-400" />
            Pending DNS
          </span>
        )}

        <button
          onClick={onRemove}
          title="Remove domain"
          className="text-muted-foreground shrink-0 p-1 opacity-0 transition hover:text-rose-300 group-hover:opacity-100"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* DNS setup hint (until it resolves here) */}
      {!domain.pointsHere && (
        <div className="mt-3 ml-12.5 flex flex-wrap items-center gap-x-2 gap-y-1 rounded-md border border-white/8 bg-black/20 px-3 py-2 text-[11px]">
          <span className="text-muted-foreground">Point a</span>
          <code className="rounded bg-white/8 px-1.5 py-0.5 font-mono text-white/80">CNAME</code>
          <span className="text-muted-foreground">to</span>
          <code className="rounded bg-white/8 px-1.5 py-0.5 font-mono text-white/80">{target}</code>
          <span className="text-muted-foreground">
            — or an <span className="font-mono text-white/70">A</span> record to your server's IP. The
            certificate issues automatically once DNS resolves here.
          </span>
        </div>
      )}
    </div>
  )
}
