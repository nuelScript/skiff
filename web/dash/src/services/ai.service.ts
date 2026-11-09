import { BaseService } from '@/services/base.service'

export type AIProviderKind = 'openai' | 'anthropic' | 'custom'

export type AIProvider = {
  name: string
  kind: AIProviderKind
  baseUrl: string
  keyHint: string
  created: number
}

export type AIUsage = { input: number; output: number; requests: number }

export type ProjectAI = {
  enabled: boolean
  budget: number
  used: AIUsage
  providers: string[]
  gateway: boolean
}

class AIService extends BaseService {
  providers() {
    return this.get<{ providers: AIProvider[]; gateway: boolean }>('/ai/providers')
  }

  saveProvider(p: { name: string; kind: AIProviderKind; baseUrl: string; key: string }) {
    return this.post('/ai/providers', p)
  }

  removeProvider(name: string) {
    return this.delete('/ai/providers', { params: { name } })
  }

  project(app: string) {
    return this.get<ProjectAI>('/project/ai', { params: { app } })
  }

  setProject(app: string, enabled: boolean, budget: number) {
    return this.put('/project/ai', { enabled, budget }, { params: { app } })
  }
}

export const aiService = new AIService()
