import { BaseService } from '@/services/base.service'

export type Disk = {
  id: string
  name: string
  path: string
  created: number
  total: number
  free: number
  missing?: boolean
}

export type MoveState = { moving: boolean; error?: string; to: string; since: number }

class DisksService extends BaseService {
  list() {
    return this.get<{ disks: Disk[]; admin: boolean }>('/disks')
  }

  add(name: string, path: string) {
    return this.post<Disk>('/disks', { name, path })
  }

  remove(id: string) {
    return this.delete('/disks', { params: { id } })
  }

  // move puts a database, or an app's volume, on a disk; an empty disk means Docker's own storage.
  move(target: { database: string } | { app: string; volume: string }, disk: string) {
    return this.post('/move', { ...target, disk })
  }
}

export const disksService = new DisksService()
