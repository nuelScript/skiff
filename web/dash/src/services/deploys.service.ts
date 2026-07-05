import { BaseService } from '@/services/base.service'

export type Deploy = {
  id: string
  app: string
  commit: string
  message: string
  trigger: string
  status: string
  started: number
  rollbackable?: boolean
}

class DeploysService extends BaseService {
  list(app: string) {
    return this.get<Deploy[]>('/deploys', { params: { app } })
  }

  // Global build feed across every app (no app filter).
  listAll() {
    return this.get<Deploy[]>('/deploys')
  }
}

export const deploysService = new DeploysService()
