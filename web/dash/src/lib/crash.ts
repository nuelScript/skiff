import type { CrashLoop } from '@/services/api.service'

// exitMeaning explains the exit codes people see most.
export function exitMeaning(code: number): string {
  switch (code) {
    case 137:
      return 'it was killed, often for running out of memory'
    case 143:
      return 'it was stopped by a signal'
    case 127:
      return "its command wasn't found"
    case 126:
      return "its command couldn't be run"
    case 0:
      return 'it exited cleanly, but a service is meant to keep running'
    default:
      return `it exited with code ${code}`
  }
}

export function crashSummary(loop: Pick<CrashLoop, 'restarts' | 'exitCode'>): string {
  return `Restarted ${loop.restarts} times in the last 10 minutes; the last time ${exitMeaning(loop.exitCode)}.`
}
