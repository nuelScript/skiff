import { BaseService } from '@/services/base.service'

export type StackPlanItem = {
  service: string
  name: string
  kind: 'app' | 'worker' | 'database'
  image?: string
  builds?: boolean
  port?: number
  public?: boolean
  replicas?: number
  volumes?: string[]
  envKeys?: string[]
  workerOf?: string
  engine?: string
}

export type StackPlan = {
  stack: string
  items: StackPlanItem[]
  problems: string[]
  notes: string[]
  unset: string[]
}

export type StackRequest = {
  name: string
  compose?: string
  git?: string
  branch?: string
  path?: string
  token?: string
  vars?: Record<string, string>
}

export type StackMember = {
  service: string
  app: string
  kind: 'app' | 'worker' | 'database'
  engine?: string
  state: string
  public?: boolean
}

export type Stack = {
  name: string
  created: number
  members: StackMember[]
}

class StacksService extends BaseService {
  list() {
    return this.get<Stack[]>('/stacks')
  }

  plan(req: StackRequest) {
    return this.post<StackPlan>('/stacks/plan', req)
  }

  create(req: StackRequest) {
    return this.post<StackPlan>('/stacks', req)
  }

  remove(name: string) {
    return this.delete('/stacks', { params: { name } })
  }
}

export const stacksService = new StacksService()
