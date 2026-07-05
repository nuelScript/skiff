import { http } from '@/config/axios.config'

export type User = { id: string; email: string; name: string; created?: number }
export type Team = { id: string; name: string; slug: string; created: number }
export type Member = { user: User; role: string }
export type Me = {
  authenticated: boolean
  needsSetup: boolean
  user?: User
  teams?: Team[]
  team?: string
}

export type App = {
  name: string
  state: string
  url: string
  repo?: string
  branch?: string
  auto: boolean
}

export type Repo = {
  full_name: string
  name: string
  private: boolean
  default_branch: string
  clone_url: string
}

export type GithubStatus = { configured: boolean; installed: boolean; slug?: string }

export type Deploy = {
  id: string
  app: string
  commit: string
  trigger: string
  status: string
  started: number
}

export type EnvVar = { key: string; value: string; build: boolean }

export type SystemInfo = {
  selfDeploy: boolean
  repo: string
  branch: string
  deploys: Deploy[]
}

export const api = {
  auth: {
    me: () => http.get<Me>('/me').then((r) => r.data),
    setup: (secret: string, email: string, name: string, password: string) =>
      http.post('/auth/setup', { secret, email, name, password }),
    login: (email: string, password: string) =>
      http.post('/auth/login', { email, password }),
    logout: () => http.post('/auth/logout'),
    accept: (token: string, name: string, password: string) =>
      http.post('/auth/accept', { token, name, password }),
    switchTeam: (team: string) => http.post('/auth/team', { team }),
    createTeam: (name: string) =>
      http.post<Team>('/teams', { name }).then((r) => r.data),
    members: () => http.get<Member[]>('/teams/members').then((r) => r.data),
    invite: (email: string, role: string) =>
      http.post<{ link: string }>('/teams/invite', { email, role }).then((r) => r.data),
  },
  system: () => http.get<SystemInfo>('/system').then((r) => r.data),
  apps: () => http.get<App[]>('/apps').then((r) => r.data),
  down: (name: string) => http.post('/down', null, { params: { app: name } }),
  deploys: (app: string) =>
    http.get<Deploy[]>('/deploys', { params: { app } }).then((r) => r.data),
  env: {
    get: (app: string) =>
      http.get<EnvVar[]>('/env', { params: { app } }).then((r) => r.data),
    set: (app: string, vars: EnvVar[]) =>
      http.put('/env', { vars }, { params: { app } }),
  },
  github: {
    status: () => http.get<GithubStatus>('/github/status').then((r) => r.data),
    repos: () => http.get<Repo[]>('/github/repos').then((r) => r.data),
  },
}
