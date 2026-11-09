import { Plus, X } from 'lucide-react'
import { Input } from '@/components/ui/input'
import type { JobStep, StepResult } from '@/services/api.service'

// StepsEditor builds a job's pipeline: each step names the steps it comes after, and runs once they've succeeded.
export function StepsEditor({ steps, onChange }: { steps: JobStep[]; onChange: (s: JobStep[]) => void }) {
  const set = (i: number, patch: Partial<JobStep>) => onChange(steps.map((s, j) => (j === i ? { ...s, ...patch } : s)))
  return (
    <div className="flex flex-col gap-2">
      {steps.length > 0 && (
        <div className="text-muted-foreground hidden gap-2 text-[11px] sm:grid sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_minmax(0,1fr)_1.75rem]">
          <span>Step</span>
          <span>Command</span>
          <span>Comes after</span>
        </div>
      )}
      {steps.map((s, i) => (
        <div key={i} className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_minmax(0,1fr)_auto]">
          <Input aria-label={`Step ${i + 1} name`} className="font-mono" placeholder="extract" value={s.name} onChange={(e) => set(i, { name: e.target.value })} />
          <Input aria-label={`Step ${i + 1} command`} className="font-mono" placeholder="python extract.py" value={s.command} onChange={(e) => set(i, { command: e.target.value })} />
          <Input
            aria-label={`Step ${i + 1} comes after`}
            className="font-mono"
            placeholder={i === 0 ? 'after: nothing' : `after: ${steps[i - 1].name || 'extract'}`}
            value={(s.after ?? []).join(', ')}
            onChange={(e) => set(i, { after: e.target.value.split(/[\s,]+/).filter(Boolean) })}
          />
          <button
            type="button"
            aria-label={`Remove step ${i + 1}`}
            onClick={() => onChange(steps.filter((_, j) => j !== i))}
            className="text-muted-foreground cursor-pointer p-1.5 transition-colors hover:text-danger"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={() => onChange([...steps, { name: '', command: '', after: steps.length ? [steps[steps.length - 1].name].filter(Boolean) : [] }])}
        className="text-muted-foreground hover:text-foreground flex w-fit cursor-pointer items-center gap-1.5 text-xs transition-colors"
      >
        <Plus className="h-3.5 w-3.5" /> Add a step
      </button>
    </div>
  )
}

// stepsLine shows a pipeline as its order, like "extract → transform, report → load".
export function stepsLine(steps: JobStep[]): string {
  const done = new Set<string>()
  const waves: string[][] = []
  let left = steps
  while (left.length) {
    const wave = left.filter((s) => (s.after ?? []).every((a) => done.has(a)))
    if (!wave.length) return steps.map((s) => s.name).join(', ')
    wave.forEach((s) => done.add(s.name))
    waves.push(wave.map((s) => s.name))
    left = left.filter((s) => !wave.includes(s))
  }
  return waves.map((w) => w.join(', ')).join(' → ')
}

const tone: Record<StepResult['status'], string> = {
  ok: 'border-success/25 text-success',
  failed: 'border-danger/30 text-danger',
  timeout: 'border-danger/30 text-danger',
  skipped: 'border-foreground/10 text-muted-foreground',
}

export function StepResults({ steps }: { steps: StepResult[] }) {
  return (
    <div className="mx-3 mb-2 flex flex-wrap gap-1.5">
      {steps.map((s) => (
        <span key={s.name} className={'rounded-md border px-1.5 py-0.5 font-mono text-[10px] ' + tone[s.status]}>
          {s.name} · {s.status === 'failed' && s.exitCode > 0 ? `exit ${s.exitCode}` : s.status}
        </span>
      ))}
    </div>
  )
}
