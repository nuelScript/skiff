import { BaseService } from '@/services/base.service'
import type { Deploy } from '@/services/deploys.service'

export type App = {
  name: string
  state: string
  url: string
  repo?: string
  branch?: string
  auto: boolean
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
  deploys: Deploy[]
}

export type ProjectSettings = {
  branch: string
  rootDir: string
  port: string
  auto: boolean
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
}

export const projectsService = new ProjectsService()
