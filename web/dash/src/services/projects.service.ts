import { BaseService } from '@/services/base.service'

export type App = {
  name: string
  state: string
  url: string
  repo?: string
  branch?: string
  auto: boolean
}

class ProjectsService extends BaseService {
  list() {
    return this.get<App[]>('/apps')
  }

  stop(name: string) {
    return this.post('/down', null, { params: { app: name } })
  }
}

export const projectsService = new ProjectsService()
