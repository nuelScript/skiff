import { BaseService } from '@/services/base.service'

export type User = { id: string; email: string; name: string; created?: number }
export type Team = { id: string; name: string; slug: string; created: number }
export type Member = { user: User; role: string }
export type Me = {
  authenticated: boolean
  needsSetup: boolean
  user?: User
  teams?: Team[]
  team?: string
  role?: string
}

class AuthService extends BaseService {
  me() {
    return this.get<Me>('/me')
  }

  setup(secret: string, email: string, name: string, password: string) {
    return this.post('/auth/setup', { secret, email, name, password })
  }

  login(email: string, password: string) {
    return this.post('/auth/login', { email, password })
  }

  logout() {
    return this.post('/auth/logout')
  }

  accept(token: string, name: string, password: string) {
    return this.post('/auth/accept', { token, name, password })
  }

  switchTeam(team: string) {
    return this.post('/auth/team', { team })
  }

  createTeam(name: string) {
    return this.post<Team>('/teams', { name })
  }

  members() {
    return this.get<Member[]>('/teams/members')
  }

  invite(email: string, role: string) {
    return this.post<{ link: string }>('/teams/invite', { email, role })
  }

  removeMember(userId: string) {
    return this.delete('/teams/members', { params: { user: userId } })
  }

  updateProfile(name: string) {
    return this.post('/account', { name })
  }

  changePassword(current: string, password: string) {
    return this.post('/account/password', { current, password })
  }

  renameTeam(name: string) {
    return this.post('/teams/rename', { name })
  }
}

export const authService = new AuthService()
