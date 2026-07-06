import { BaseService } from '@/services/base.service'
import type { Deploy } from '@/services/deploys.service'

export type App = {
  name: string
  state: string
  url: string
  repo?: string
  branch?: string
  auto: boolean
  commit?: string
  message?: string
  updated?: number
}

export type ProjectDetail = {
  name: string
  state: string
  url: string
  repo: string
  branch: string
  rootDir: string
  port: string
  auto: boolean
  previewAuto: boolean
  replicas: number
  release: string
  deploys: Deploy[]
  previews: Preview[]
}

export type Job = {
  id: string
  app: string
  name: string
  schedule: string
  command: string
  lastRun: number
  lastOk: boolean
  next: number
  created: number
}

export type Preview = {
  name: string
  branch: string
  url: string
  state: string
  status: string
  updated: number
}

export type ProjectSettings = {
  branch: string
  rootDir: string
  port: string
  auto: boolean
  previewAuto: boolean
  replicas: number
  release: string
}

class ProjectsService extends BaseService {
  list() {
    return this.get<App[]>('/apps')
  }

  detail(name: string) {
    return this.get<ProjectDetail>('/project', { params: { app: name } })
  }

  update(name: string, settings: ProjectSettings) {
    return this.put('/project', settings, { params: { app: name } })
  }

  stop(name: string) {
    return this.post('/down', null, { params: { app: name } })
  }

  jobs(app: string) {
    return this.get<Job[]>('/jobs', { params: { app } })
  }

  createJob(app: string, name: string, schedule: string, command: string) {
    return this.post<Job>('/jobs', { name, schedule, command }, { params: { app } })
  }

  deleteJob(id: string) {
    return this.delete('/jobs', { params: { id } })
  }

  runJob(id: string) {
    return this.post<{ ok: boolean; output: string }>('/jobs/run', null, { params: { id } })
  }
}

export const projectsService = new ProjectsService()
