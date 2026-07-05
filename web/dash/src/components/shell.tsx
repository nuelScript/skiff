import { useState } from 'react'
import { Outlet } from 'react-router'
import { useAuthContext } from '@/lib/auth-context'
import { api } from '@/services/api.service'
import { SidebarProvider, SidebarInset } from '@/components/ui/sidebar'
import AppSidebar from '@/components/app-sidebar'
import Topbar from '@/components/topbar'
import MembersDialog from '@/components/members-dialog'

// The authenticated frame: a persistent sidebar + top bar around the routed page.
export default function Shell() {
  const { me, switchTeam, logout } = useAuthContext()
  const [members, setMembers] = useState(false)

  const createTeam = async () => {
    const name = window.prompt('New team name')
    if (name && name.trim()) {
      const t = await api.auth.createTeam(name.trim())
      await switchTeam(t.id)
    }
  }

  return (
    <SidebarProvider>
      <AppSidebar
        me={me!}
        switchTeam={switchTeam}
        logout={logout}
        onMembers={() => setMembers(true)}
        onCreateTeam={createTeam}
      />
      <SidebarInset className="relative bg-transparent">
        <div className="skiff-ambient pointer-events-none absolute inset-0 -z-10" />
        <Topbar />
        <Outlet />
      </SidebarInset>
      <MembersDialog open={members} onOpenChange={setMembers} />
    </SidebarProvider>
  )
}
