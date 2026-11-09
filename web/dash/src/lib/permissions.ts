import type { Permissions } from '@/services/api.service'

export const splitList = (s: string): string[] =>
  s
    .split(/[\s,]+/)
    .map((v) => v.trim())
    .filter(Boolean)

export function isEmpty(p: Permissions): boolean {
  return !p.privileged && !p.hostNetwork && !p.capabilities?.length && !p.devices?.length && !p.sysctls?.length
}

// describe lists a grant in plain words, one phrase per kind.
export function describe(p: Permissions): string[] {
  const out: string[] = []
  if (p.privileged) out.push('privileged mode')
  if (p.hostNetwork) out.push("the server's own network")
  if (p.capabilities?.length) out.push(`capabilities ${p.capabilities.join(', ')}`)
  if (p.devices?.length) out.push(`devices ${p.devices.join(', ')}`)
  if (p.sysctls?.length) out.push(`sysctls ${p.sysctls.join(', ')}`)
  return out
}

// merge adds what an app asks for to what it already has.
export function merge(a: Permissions, b: Permissions): Permissions {
  const both = (x?: string[], y?: string[]) => [...new Set([...(x ?? []), ...(y ?? [])])]
  return {
    capabilities: both(a.capabilities, b.capabilities),
    devices: both(a.devices, b.devices),
    sysctls: both(a.sysctls, b.sysctls),
    privileged: !!(a.privileged || b.privileged),
    hostNetwork: !!(a.hostNetwork || b.hostNetwork),
  }
}
