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
  // Keyset-paginated newest-first. Omit `before` for the first page; pass the
  // last entry's id to page into older history.
  list(before?: number, limit = 30) {
    return this.get<AuditEntry[]>('/audit', {
      params: before ? { limit, before } : { limit },
    })
  }
}

export const auditService = new AuditService()
