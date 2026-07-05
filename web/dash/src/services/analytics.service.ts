import { BaseService } from '@/services/base.service'

export type AnalyticsPoint = { t: number; req: number }
export type AnalyticsApp = { name: string; req: number; avgLatMs: number }

export type Analytics = {
  windowMins: number
  total: number
  status: { s2: number; s3: number; s4: number; s5: number }
  series: AnalyticsPoint[]
  apps: AnalyticsApp[]
  updated: number
}

class AnalyticsService extends BaseService {
  overview() {
    return this.get<Analytics>('/analytics')
  }
}

export const analyticsService = new AnalyticsService()
