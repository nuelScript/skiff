import { BaseService } from '@/services/base.service'

export type AIProviderKind = 'openai' | 'anthropic' | 'custom'

export type AIProvider = {
  name: string
  kind: AIProviderKind
  baseUrl: string
  keyHint: string
  created: number
}

export type AIPrice = { model: string; input: number; output: number }

export type AIUsage = { input: number; output: number; requests: number }

export type ProjectAI = {
  enabled: boolean
  budget: number
  runBudget: number
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
  gave_up: boolean
  done: boolean
}

export type Sandbox = {
  id: number
  image: string
  command: string
  internet: boolean
  files: number
  started: number
  finished: number
  status: 'running' | 'ok' | 'failed' | 'timeout' | 'stopped'
  exitCode: number
  output: string
}

export type TraceSummary = {
  id: string
  name: string
  start: number
  durationMs: number
  spans: number
  errors: number
  inputTokens: number
  outputTokens: number
  modelCalls: number
  toolCalls: number
  cost?: number
  costPartial?: boolean
}

export type SpanView = {
  id: string
  parent: string
  name: string
  startNs: number
  endNs: number
  status: number
  message?: string
  model?: string
  tool?: string
  inputTokens?: number
  outputTokens?: number
  source: 'app' | 'gateway'
  attrs: Record<string, unknown>
  cost?: number
}

export type Approval = {
  id: number
  action: string
  detail: string
  status: 'pending' | 'approved' | 'denied' | 'expired'
  note?: string
  by?: string
  created: number
  answered?: number
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
      sandboxes: Sandbox[]
      statePath: string
    }

class AIService extends BaseService {
  providers() {
    return this.get<{ providers: AIProvider[]; gateway: boolean }>('/ai/providers')
  }

  saveProvider(p: { name: string; kind: AIProviderKind; baseUrl: string; key: string }) {
    return this.post('/ai/providers', p)
  }

  prices() {
    return this.get<{ prices: AIPrice[] }>('/ai/prices')
  }

  savePrice(p: AIPrice) {
    return this.put('/ai/prices', p)
  }

  removePrice(model: string) {
    return this.delete('/ai/prices', { params: { model } })
  }

  removeProvider(name: string) {
    return this.delete('/ai/providers', { params: { name } })
  }

  project(app: string) {
    return this.get<ProjectAI>('/project/ai', { params: { app } })
  }

  setProject(app: string, enabled: boolean, budget: number, runBudget?: number) {
    return this.put('/project/ai', { enabled, budget, runBudget }, { params: { app } })
  }

  agent(app: string) {
    return this.get<Agent>('/project/agent', { params: { app } })
  }

  setAgentSchedule(app: string, schedule: string) {
    return this.put('/project/agent', { schedule }, { params: { app } })
  }

  traces(app: string) {
    return this.get<{ traces: TraceSummary[] }>('/project/traces', { params: { app } })
  }

  trace(app: string, trace: string) {
    return this.get<{ spans: SpanView[] }>('/project/traces', { params: { app, trace } })
  }

  approvals(app: string) {
    return this.get<{ approvals: Approval[] }>('/project/approvals', { params: { app } })
  }

  answerApproval(app: string, id: number, approve: boolean, note: string) {
    return this.post('/project/approvals', { id, approve, note }, { params: { app } })
  }

  agentAction(app: string, action: 'stop' | 'start') {
    return this.post('/project/agent', undefined, { params: { app, action } })
  }
}

export const aiService = new AIService()
