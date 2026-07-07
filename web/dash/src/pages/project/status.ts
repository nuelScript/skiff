// Status → tailwind class helpers shared by the project page and its panels.

export const runningPill = (state: string): string =>
  state === 'running'
    ? 'border-emerald-400/25 bg-emerald-400/10 text-emerald-300'
    : state === 'exited' || state === 'missing'
      ? 'border-rose-400/25 bg-rose-400/10 text-rose-300'
      : 'border-white/15 bg-white/5 text-muted-foreground'

export const deployDot = (status: string): string =>
  status === 'live'
    ? 'bg-emerald-400'
    : status === 'failed'
      ? 'bg-rose-400'
      : status === 'canceled'
        ? 'bg-white/25'
        : 'bg-amber-400 pulse-dot'
