import { BaseService } from '@/services/base.service'

export type AnalyticsSeries = {
  t: number
  req: number
  s2: number
  s3: number
  s4: number
  s5: number
  bi: number
  bo: number
  lat: number
}

export type AnalyticsApp = { name: string; req: number; avgLatMs: number }

export type Analytics = {
  rangeMins: number
  bucketSecs: number
  total: number
  status: { s2: number; s3: number; s4: number; s5: number }
  bytesIn: number
  bytesOut: number
  avgLatMs: number
  series: AnalyticsSeries[]
  apps: AnalyticsApp[]
  appOptions: string[]
  updated: number
}

class AnalyticsService extends BaseService {
  overview(rangeMins: number, app: string) {
    return this.get<Analytics>('/analytics', {
      params: { range: rangeMins, app: app || undefined },
    })
  }
}

export const analyticsService = new AnalyticsService()
