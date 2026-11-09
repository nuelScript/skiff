import { BaseService } from '@/services/base.service'

export type ApiToken = {
  id: string
  name: string
  created: number
  lastUsed: number
  token?: string // present only in the create response, shown once
  scopes?: TokenScope[]
  apps?: string[]
}

export type TokenScope = 'read' | 'deploy' | 'env' | 'work' | 'data'

// SCOPE_WORDS name each scope in a token's summary line.
export const SCOPE_WORDS: Record<TokenScope, string> = {
  read: 'read',
  deploy: 'deploy',
  env: 'environment variables',
  work: 'workers, jobs and runs',
  data: 'data',
}

export const TOKEN_SCOPES: { value: TokenScope; label: string }[] = [
  { value: 'read', label: 'Read apps, deploys and logs' },
  { value: 'deploy', label: 'Deploy and roll back' },
  { value: 'env', label: 'Read and change environment variables' },
  { value: 'work', label: 'Change workers and jobs, and run commands' },
  { value: 'data', label: 'Databases, buckets and volumes' },
]

class TokensService extends BaseService {
  list() {
    return this.get<ApiToken[]>('/tokens')
  }

  create(name: string, scopes: TokenScope[], apps: string[]) {
    return this.post<ApiToken>('/tokens', { name, scopes, apps })
  }

  revoke(id: string) {
    return this.delete('/tokens', { params: { id } })
  }
}

export const tokensService = new TokensService()
