import { BaseService } from '@/services/base.service'

export type Deploy = {
  id: string
  app: string
  commit: string
  trigger: string
  status: string
  started: number
}

class DeploysService extends BaseService {
  list(app: string) {
    return this.get<Deploy[]>('/deploys', { params: { app } })
  }
}

export const deploysService = new DeploysService()
