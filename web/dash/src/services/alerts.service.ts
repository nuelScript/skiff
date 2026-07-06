import { BaseService } from '@/services/base.service'

export type AlertConfig = {
  email: string
  slackUrl: string
  webhookUrl: string
  smtp: boolean // whether the server has SMTP configured (email deliverable)
}

export type AlertTestResult = { channel: string; ok: boolean; error?: string }

class AlertsService extends BaseService {
  config() {
    return this.get<AlertConfig>('/alerts')
  }

  save(cfg: { email: string; slackUrl: string; webhookUrl: string }) {
    return this.put('/alerts', cfg)
  }

  test() {
    return this.post<{ results: AlertTestResult[] }>('/alerts/test', null)
  }
}

export const alertsService = new AlertsService()
