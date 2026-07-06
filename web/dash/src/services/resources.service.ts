import { BaseService } from '@/services/base.service'

export type ResourcesSeries = { t: number; cpu: number; mem: number }

export type ResourcesApp = { name: string; cpu: number; mem: number }

export type Resources = {
  rangeMins: number
  bucketSecs: number
  curCpu: number
  curMem: number
  peakCpu: number
  peakMem: number
  memLimit: number
  restarts: number
  samples: number
  series: ResourcesSeries[]
  apps: ResourcesApp[]
  appOptions: string[]
  updated: number
}

class ResourcesService extends BaseService {
  overview(rangeMins: number, app: string) {
    return this.get<Resources>('/resources', {
      params: { range: rangeMins, app: app || undefined },
    })
  }
}

export const resourcesService = new ResourcesService()
