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

export type AgentEvent = {
  id: number
  source: string
  kind: string
  body: string
  created: number
  deliveries: number
  done: boolean
}

export type Agent =
  | { agent: false }
  | {
      agent: true
      stopped: boolean
      schedule: string
      next: number
      webhook: string
      webhookSecret: string
      pending: number
      events: AgentEvent[]
      statePath: string
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

  agent(app: string) {
    return this.get<Agent>('/project/agent', { params: { app } })
  }

  setAgentSchedule(app: string, schedule: string) {
    return this.put('/project/agent', { schedule }, { params: { app } })
  }

  agentAction(app: string, action: 'stop' | 'start') {
    return this.post('/project/agent', undefined, { params: { app, action } })
  }
}

export const aiService = new AIService()
