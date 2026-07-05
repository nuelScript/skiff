import { BaseService } from '@/services/base.service'

export type EnvVar = { key: string; value: string; build: boolean }

class EnvService extends BaseService {
  list(app: string) {
    return this.get<EnvVar[]>('/env', { params: { app } })
  }

  save(app: string, vars: EnvVar[]) {
    return this.put('/env', { vars }, { params: { app } })
  }
}

export const envService = new EnvService()
