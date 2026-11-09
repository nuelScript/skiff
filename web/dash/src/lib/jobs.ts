import type { Job, JobRun } from '@/services/api.service'

export const runStatusLabel: Record<JobRun['status'], string> = {
  queued: 'Waiting',
  running: 'Running',
  ok: 'Succeeded',
  failed: 'Failed',
  timeout: 'Timed out',
  skipped: 'Skipped',
}

export const runStatusTone: Record<JobRun['status'], string> = {
  queued: 'text-muted-foreground',
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
    case 'webhook':
      return 'by its webhook'
    case 'bucket':
      return 'for a new file'
    default:
      return 'on schedule'
  }
}

// jobTriggers says what starts a job besides its schedule, which the row shows on its own.
export function jobTriggers(j: Job): string {
  const parts: string[] = []
  if (j.webhook) parts.push('when its webhook is called')
  if (j.bucket) parts.push(`for each new file in ${j.bucket}/${j.prefix ?? ''}`)
  return parts.join(', ')
}

// jobSettingsSummary says how a job behaves when something goes wrong, in one line.
export function jobSettingsSummary(j: Job): string {
  const parts = [`times out after ${timeoutLabel(j.timeout)}`]
  if (j.retries > 0) parts.push(j.retries === 1 ? 'retries once' : `retries ${j.retries} times`)
  if (j.schedule)
    parts.push(j.catchUp === 'skip' ? 'skips runs missed while Skiff was down' : 'catches up once after downtime')
  return parts.join(' · ')
}

function timeoutLabel(seconds: number): string {
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  return `${Math.round(seconds / 60)} min`
}
