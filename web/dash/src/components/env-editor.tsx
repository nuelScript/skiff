import { useEffect, useState } from 'react'
import { Plus, X } from 'lucide-react'
import { envService, type EnvVar } from '@/services/api.service'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

const blank = (): EnvVar => ({ key: '', value: '', build: false })

// The environment-variable editor: load, edit, add/remove, save. Reused inline
// on the project page and inside the env dialog.
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

  const update = (i: number, patch: Partial<EnvVar>) =>
    setVars((vs) => vs.map((v, idx) => (idx === i ? { ...v, ...patch } : v)))

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
        Injected on the next deploy. <b className="text-foreground/80">Build</b> vars are
        available at build time; the rest are runtime-only secrets, never baked into the image.
      </p>

      <div className="flex flex-col gap-2">
        {vars.map((v, i) => (
          <div key={i} className="flex items-center gap-2">
            <Input
              placeholder="KEY"
              value={v.key}
              onChange={(e) => update(i, { key: e.target.value })}
              className="font-mono"
            />
            <Input
              placeholder="value"
              value={v.value}
              onChange={(e) => update(i, { value: e.target.value })}
              className="font-mono"
            />
            <button
              type="button"
              title="Available at build time"
              onClick={() => update(i, { build: !v.build })}
              className={
                'shrink-0 rounded-md border px-2 py-2 font-mono text-[10px] uppercase ' +
                (v.build ? 'text-foreground' : 'text-muted-foreground')
              }
            >
              build
            </button>
            <button
              type="button"
              onClick={() => setVars((vs) => vs.filter((_, idx) => idx !== i))}
              className="text-muted-foreground hover:text-foreground shrink-0"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        ))}
        <button
          type="button"
          onClick={() => setVars((vs) => [...vs, blank()])}
          className="text-muted-foreground hover:text-foreground flex items-center gap-1.5 self-start text-sm"
        >
          <Plus className="h-3.5 w-3.5" /> Add variable
        </button>
      </div>

      <div className="flex items-center justify-end gap-3">
        {saved && (
          <span className="text-muted-foreground text-xs">Saved — redeploy to apply.</span>
        )}
        <Button onClick={save}>Save</Button>
      </div>
    </div>
  )
}
