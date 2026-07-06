import { BaseService } from '@/services/base.service'

export type Bucket = {
  id: string
  name: string
  endpoint: string
  region: string
  accessKey: string
  secretKey: string
  state: string
  attached: string[]
  created: number
}

class StorageService extends BaseService {
  list() {
    return this.get<Bucket[]>('/storage')
  }

  create(name: string) {
    return this.post<Bucket>('/storage', { name })
  }

  remove(id: string) {
    return this.delete('/storage', { params: { id } })
  }

  attach(id: string, app: string) {
    return this.post('/storage/attach', undefined, { params: { id, app } })
  }

  detach(id: string, app: string) {
    return this.delete('/storage/attach', { params: { id, app } })
  }
}

export const storageService = new StorageService()
