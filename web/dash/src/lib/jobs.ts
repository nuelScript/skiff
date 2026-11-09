import type { Job, JobRun } from '@/services/api.service'

export const runStatusLabel: Record<JobRun['status'], string> = {
  running: 'Running',
  ok: 'Succeeded',
  failed: 'Failed',
  timeout: 'Timed out',
  skipped: 'Skipped',
}

export const runStatusTone: Record<JobRun['status'], string> = {
  running: 'text-foreground',
  ok: 'text-success',
  failed: 'text-danger',
  timeout: 'text-danger',
  skipped: 'text-muted-foreground',
}

export function runTrigger(r: JobRun): string {
  switch (r.trigger) {
    case 'catch-up':
      return 'catching up after downtime'
    case 'manual':
      return 'run by hand'
    case 'retry':
      return `retry ${r.attempt - 1}`
    default:
      return 'on schedule'
  }
}

// jobSettingsSummary says how a job behaves when something goes wrong, in one line.
export function jobSettingsSummary(j: Job): string {
  const parts = [`times out after ${timeoutLabel(j.timeout)}`]
  if (j.retries > 0) parts.push(j.retries === 1 ? 'retries once' : `retries ${j.retries} times`)
  parts.push(j.catchUp === 'skip' ? 'skips runs missed while Skiff was down' : 'catches up once after downtime')
  return parts.join(' · ')
}

function timeoutLabel(seconds: number): string {
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  return `${Math.round(seconds / 60)} min`
}
