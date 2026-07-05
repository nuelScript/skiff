import { useEffect, useState, type ClipboardEvent } from 'react'
import { Plus, X, Check } from 'lucide-react'
import { envService, type EnvVar } from '@/services/api.service'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

const blank = (): EnvVar => ({ key: '', value: '', build: false })

// Parse pasted .env content: KEY=value lines, tolerating `export`, quotes,
// `#` comments, and blank lines.
function parseEnv(text: string): EnvVar[] {
  const out: EnvVar[] = []
  for (const raw of text.split(/\r?\n/)) {
    let line = raw.trim()
    if (!line || line.startsWith('#')) continue
    if (line.startsWith('export ')) line = line.slice(7).trimStart()
    const eq = line.indexOf('=')
    if (eq < 1) continue
    const key = line.slice(0, eq).trim()
    let value = line.slice(eq + 1).trim()
    const quote = value[0]
    if ((quote === '"' || quote === "'") && value.endsWith(quote) && value.length > 1) {
      value = value.slice(1, -1)
    }
    if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) out.push({ key, value, build: false })
  }
  return out
}

// Merge pasted vars over the current ones, keyed by name (paste wins).
function merge(existing: EnvVar[], pasted: EnvVar[]): EnvVar[] {
  const map = new Map<string, EnvVar>()
  for (const v of existing) if (v.key.trim()) map.set(v.key, v)
  for (const p of pasted) map.set(p.key, p)
  const out = Array.from(map.values())
  return out.length ? out : [blank()]
}

export function EnvEditor({ app }: { app: string }) {
  const [vars, setVars] = useState<EnvVar[]>([blank()])
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    setSaved(false)
    envService
      .list(app)
      .then((v) => setVars(v.length ? v : [blank()]))
      .catch(() => setVars([blank()]))
  }, [app])

  const update = (i: number, patch: Partial<EnvVar>) => {
    setSaved(false)
    setVars((vs) => vs.map((v, idx) => (idx === i ? { ...v, ...patch } : v)))
  }

  // Pasting a KEY=value (or a whole .env) into a name field fills the rows.
  const onPaste = (e: ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData('text')
    if (!/[\n=]/.test(text)) return
    const parsed = parseEnv(text)
    if (parsed.length === 0) return
    e.preventDefault()
    setSaved(false)
    setVars((vs) => merge(vs, parsed))
  }

  const save = async () => {
    await envService.save(
      app,
      vars.filter((v) => v.key.trim()),
    )
    setSaved(true)
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="text-muted-foreground text-sm">
        Injected on the next deploy. <b className="text-foreground/80">Build</b> vars are available
        during the build; the rest are runtime-only secrets, never baked into the image. Tip: paste
        a <span className="text-foreground/70 font-mono">.env</span> straight into a name field to
        fill everything at once.
      </p>

      <div className="divide-y divide-white/5 overflow-hidden rounded-lg border border-white/8">
        <div className="text-muted-foreground grid grid-cols-[1fr_1.5fr_auto] gap-3 px-3 py-2 font-mono text-[10px] tracking-wider uppercase">
          <span>Name</span>
          <span>Value</span>
          <span className="w-[92px]" />
        </div>
        {vars.map((v, i) => (
          <div
            key={i}
            className="grid grid-cols-[1fr_1.5fr_auto] items-center gap-3 px-3 focus-within:bg-white/[0.02]"
          >
            <Input
              placeholder="EXAMPLE_KEY"
              value={v.key}
              onChange={(e) => update(i, { key: e.target.value })}
              onPaste={onPaste}
              className="h-10 border-0 bg-transparent px-0 font-mono text-sm shadow-none focus-visible:ring-0"
            />
            <Input
              placeholder="value"
              value={v.value}
              onChange={(e) => update(i, { value: e.target.value })}
              className="h-10 border-0 bg-transparent px-0 font-mono text-sm shadow-none focus-visible:ring-0"
            />
            <div className="flex w-[92px] items-center justify-end gap-1">
              <button
                type="button"
                title="Available at build time"
                onClick={() => update(i, { build: !v.build })}
                className={
                  'rounded-[5px] border px-2 py-1 font-mono text-[10px] tracking-wide uppercase transition-colors ' +
                  (v.build
                    ? 'border-emerald-400/30 bg-emerald-400/10 text-emerald-300'
                    : 'border-white/12 text-muted-foreground hover:text-foreground')
                }
              >
                build
              </button>
              <button
                type="button"
                title="Remove"
                onClick={() => setVars((vs) => (vs.length > 1 ? vs.filter((_, idx) => idx !== i) : [blank()]))}
                className="text-muted-foreground hover:text-foreground rounded-[5px] p-1.5"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        ))}
      </div>

      <div className="flex items-center justify-between">
        <button
          type="button"
          onClick={() => setVars((vs) => [...vs, blank()])}
          className="text-muted-foreground hover:text-foreground flex items-center gap-1.5 text-sm transition-colors"
        >
          <Plus className="h-3.5 w-3.5" /> Add variable
        </button>
        <div className="flex items-center gap-3">
          {saved && (
            <span className="flex items-center gap-1.5 text-xs text-emerald-300">
              <Check className="h-3.5 w-3.5" /> Saved — redeploy to apply.
            </span>
          )}
          <Button size="sm" onClick={save}>
            Save
          </Button>
        </div>
      </div>
    </div>
  )
}
