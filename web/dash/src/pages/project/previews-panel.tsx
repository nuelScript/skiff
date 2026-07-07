import { useState, type FormEvent } from 'react'
import { ExternalLink, GitBranch, Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useConfirm } from '@/providers/confirm-provider'
import { type Preview } from '@/services/api.service'
import { deployDot } from './status'

export function PreviewsPanel({
  project,
  previews,
  onCreate,
  onTeardown,
}: {
  project: string
  previews: Preview[]
  onCreate: (branch: string) => void
  onTeardown: (name: string) => void | Promise<void>
}) {
  const [branch, setBranch] = useState('')
  const create = (e: FormEvent) => {
    e.preventDefault()
    const b = branch.trim()
    if (!b) return
    onCreate(b)
    setBranch('')
  }
  return (
    <div className="max-w-3xl space-y-4">
      <form
        onSubmit={create}
        className="rounded-xl border border-white/10 bg-linear-to-b from-white/3 to-transparent p-4"
      >
        <h2 className="text-sm font-medium">New preview environment</h2>
        <p className="text-muted-foreground mt-1 mb-3 text-xs">
          Deploy any branch of <span className="text-foreground/70 font-mono">{project}</span> to its
          own live URL with its own certificate. Pushes to the branch redeploy it automatically.
        </p>
        <div className="flex flex-col gap-2 sm:flex-row">
          <div className="relative min-w-0 flex-1">
            <GitBranch className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-3.5 w-3.5 -translate-y-1/2" />
            <input
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
              placeholder="branch name — e.g. feat/login"
              className="h-9 w-full rounded-[6px] border border-white/12 bg-black/30 pr-3 pl-9 font-mono text-sm outline-none placeholder:text-white/25 focus-visible:border-white/30"
            />
          </div>
          <Button type="submit" size="sm" disabled={!branch.trim()} className="shrink-0">
            <Plus className="h-4 w-4" />
            Create preview
          </Button>
        </div>
      </form>

      {previews.length === 0 ? (
        <div className="text-muted-foreground flex flex-col items-center gap-2 rounded-xl border border-white/8 py-12 text-sm">
          <GitBranch className="h-5 w-5 opacity-40" />
          <span>No preview environments yet.</span>
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-white/8">
          {previews.map((pv) => (
            <PreviewRow key={pv.name} pv={pv} onTeardown={() => onTeardown(pv.name)} />
          ))}
        </div>
      )}
    </div>
  )
}

function PreviewRow({ pv, onTeardown }: { pv: Preview; onTeardown: () => void }) {
  const confirm = useConfirm()
  const host = pv.url.replace(/^https?:\/\//, '')
  const label = pv.state === 'running' ? 'Ready' : pv.status || pv.state
  return (
    <div className="group flex items-center gap-3.5 border-b border-white/5 px-4 py-3 last:border-0 hover:bg-white/2">
      <span className={'h-2 w-2 shrink-0 rounded-full ' + deployDot(pv.status)} />
      <div className="min-w-0 flex-1">
        <a
          href={pv.url}
          target="_blank"
          rel="noreferrer"
          className="hover:text-foreground group/link flex items-center gap-1.5 text-sm font-medium"
        >
          <span className="truncate font-mono">{host}</span>
          <ExternalLink className="h-3 w-3 shrink-0 opacity-0 transition group-hover/link:opacity-60" />
        </a>
        <p className="text-muted-foreground mt-0.5 flex items-center gap-1 truncate text-xs">
          <GitBranch className="h-3 w-3 shrink-0" /> {pv.branch}
        </p>
      </div>
      <span className="text-muted-foreground shrink-0 text-[11px] capitalize">{label}</span>
      <button
        onClick={async () => {
          if (
            await confirm({
              title: `Tear down ${host}?`,
              description: 'This removes the preview and its build.',
              confirmText: 'Tear down',
              destructive: true,
            })
          )
            onTeardown()
        }}
        title="Tear down preview"
        className="text-muted-foreground shrink-0 p-1 opacity-0 transition hover:text-rose-300 group-hover:opacity-100"
      >
        <Trash2 className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}
