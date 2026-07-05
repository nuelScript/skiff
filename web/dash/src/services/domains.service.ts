import { BaseService } from '@/services/base.service'

export type Domain = {
  host: string
  app: string
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

  add(app: string, host: string) {
    return this.post<Domain>('/domains', { app, host })
  }

  remove(host: string) {
    return this.delete('/domains', { params: { host } })
  }
}

export const domainsService = new DomainsService()
