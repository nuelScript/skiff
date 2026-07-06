// Centralized TanStack Query keys so every query and its invalidations stay in
// sync. Reference these — never inline a key string.
export const queryKeys = {
  me: ['me'] as const,
  apps: ['apps'] as const,
  project: (name: string) => ['project', name] as const,
  system: ['system'] as const,
  server: ['server'] as const,
  domains: ['domains'] as const,
  databases: ['databases'] as const,
  members: ['members'] as const,
  analytics: ['analytics'] as const,
  resources: ['resources'] as const,
  alerts: ['alerts'] as const,
  audit: ['audit'] as const,
  deploys: (app: string | null) => ['deploys', app] as const,
  deploysAll: ['deploys', 'all'] as const,
  github: {
    status: ['github', 'status'] as const,
    repos: ['github', 'repos'] as const,
  },
} as const
