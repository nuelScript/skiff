import { BaseService } from '@/services/base.service'

export type ResourceUsage = { total: number; used: number }

export type ContainerStat = {
  name: string
  image: string
  cpuPct: number
  memUsed: number
  memPct: number
}

export type ServerInfo = {
  hostname: string
  os: string
  uptime: number
  load: number[]
  cpuCount: number
  cpuPct: number
  mem: ResourceUsage
  disk: ResourceUsage
  docker: string
  containers: ContainerStat[]
}

class ServerService extends BaseService {
  info() {
    return this.get<ServerInfo>('/server')
  }
}

export const serverService = new ServerService()
