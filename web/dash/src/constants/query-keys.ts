// Centralized TanStack Query keys so every query and its invalidations stay in
// sync. Reference these — never inline a key string.
export const queryKeys = {
  me: ['me'] as const,
  apps: ['apps'] as const,
  project: (name: string) => ['project', name] as const,
  system: ['system'] as const,
  server: ['server'] as const,
  deploys: (app: string | null) => ['deploys', app] as const,
  github: {
    status: ['github', 'status'] as const,
    repos: ['github', 'repos'] as const,
  },
} as const
