import { BaseService } from '@/services/base.service'

export type AuditEntry = {
  id: number
  actor: string
  action: string
  target: string
  detail: string
  created: number
}

class AuditService extends BaseService {
  list() {
    return this.get<AuditEntry[]>('/audit')
  }
}

export const auditService = new AuditService()
