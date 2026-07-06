import { BaseService } from '@/services/base.service'

export type DbEngine = 'postgres' | 'mysql' | 'mongodb' | 'redis'

export type Database = {
  id: string
  name: string
  engine: DbEngine
  host: string
  port: number
  created: number
  state: string
  url: string
  attached: string[]
  public: boolean
  publicUrl?: string
}

class DatabasesService extends BaseService {
  list() {
    return this.get<Database[]>('/databases')
  }

  create(engine: DbEngine, name: string) {
    return this.post<Database>('/databases', { engine, name })
  }

  remove(id: string) {
    return this.delete('/databases', { params: { id } })
  }

  attach(id: string, app: string) {
    return this.post<Database>('/databases/attach', undefined, { params: { id, app } })
  }

  setPublic(id: string, on: boolean) {
    return this.post<Database>('/databases/public', undefined, { params: { id, on: on ? '1' : '0' } })
  }

  detach(id: string, app: string) {
    return this.delete<Database>('/databases/attach', { params: { id, app } })
  }
}

export const databasesService = new DatabasesService()
