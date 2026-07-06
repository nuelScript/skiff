import { BaseService } from '@/services/base.service'

export type ApiToken = {
  id: string
  name: string
  created: number
  lastUsed: number
  token?: string // present only in the create response, shown once
}

class TokensService extends BaseService {
  list() {
    return this.get<ApiToken[]>('/tokens')
  }

  create(name: string) {
    return this.post<ApiToken>('/tokens', { name })
  }

  revoke(id: string) {
    return this.delete('/tokens', { params: { id } })
  }
}

export const tokensService = new TokensService()
