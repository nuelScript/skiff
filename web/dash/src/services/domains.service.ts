import { BaseService } from '@/services/base.service'

export type Domain = {
  host: string
  app: string
  created: number
  pointsHere?: boolean
}

class DomainsService extends BaseService {
  // All custom domains across the team's apps.
  list() {
    return this.get<Domain[]>('/domains')
  }

  add(app: string, host: string) {
    return this.post<Domain>('/domains', { app, host })
  }

  remove(host: string) {
    return this.delete('/domains', { params: { host } })
  }
}

export const domainsService = new DomainsService()
