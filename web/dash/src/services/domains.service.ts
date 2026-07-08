import { BaseService } from '@/services/base.service'

export type Domain = {
  host: string
  app: string
  parent?: string
  branch?: string
  created: number
  pointsHere?: boolean
  resolvesTo?: string[]
}

export type DomainsResponse = {
  serverIp: string
  domains: Domain[]
}

class DomainsService extends BaseService {
  list() {
    return this.get<DomainsResponse>('/domains')
  }

  add(app: string, host: string, branch?: string) {
    return this.post<Domain>('/domains', branch ? { app, host, branch } : { app, host })
  }

  remove(host: string) {
    return this.delete('/domains', { params: { host } })
  }
}

export const domainsService = new DomainsService()
