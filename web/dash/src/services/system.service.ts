import { BaseService } from '@/services/base.service'
import type { Deploy } from '@/services/deploys.service'

export type SystemInfo = {
  selfDeploy: boolean
  repo: string
  branch: string
  deploys: Deploy[]
}

class SystemService extends BaseService {
  info() {
    return this.get<SystemInfo>('/system')
  }
}

export const systemService = new SystemService()
