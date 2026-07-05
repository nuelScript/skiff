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

  // Stop a build — cancels the live one, or force-clears a stuck one. Pass an
  // empty id to cancel whatever is currently building for the app.
  cancel(app: string, id: string) {
    return this.post('/cancel', null, { params: { app, id } })
  }
}

export const deploysService = new DeploysService()
