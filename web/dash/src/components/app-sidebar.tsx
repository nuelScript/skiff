import type { ComponentType } from 'react'
import { NavLink, useLocation } from 'react-router'
import {
  LayoutGrid,
  Rocket,
  ScrollText,
  Server,
  Globe,
  KeyRound,
  Settings,
} from 'lucide-react'
import type { Me } from '@/services/api.service'
import { Logo } from '@/components/logo'
import TeamSwitcher from '@/components/team-switcher'
import UserMenu from '@/components/user-menu'
import {
  Sidebar,
  SidebarHeader,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
} from '@/components/ui/sidebar'

type NavItem = {
  to: string
  label: string
  icon: ComponentType<{ className?: string }>
  end?: boolean
}

const nav: NavItem[] = [
  { to: '/', label: 'Projects', icon: LayoutGrid, end: true },
  { to: '/deployments', label: 'Deployments', icon: Rocket },
  { to: '/logs', label: 'Logs', icon: ScrollText },
  { to: '/server', label: 'Server', icon: Server },
  { to: '/domains', label: 'Domains', icon: Globe },
  { to: '/env', label: 'Environment', icon: KeyRound },
  { to: '/settings', label: 'Settings', icon: Settings },
]

export default function AppSidebar({
  me,
  switchTeam,
  logout,
  onMembers,
  onCreateTeam,
}: {
  me: Me
  switchTeam: (id: string) => Promise<void>
  logout: () => void
  onMembers: () => void
  onCreateTeam: () => void
}) {
  const { pathname } = useLocation()
  const active = (to: string, end?: boolean) =>
    end ? pathname === to : pathname === to || pathname.startsWith(to + '/')

  return (
    <Sidebar>
      <SidebarHeader className="gap-3">
        <div className="flex items-center px-2 pt-1">
          <Logo />
        </div>
        <TeamSwitcher
          me={me}
          switchTeam={switchTeam}
          onMembers={onMembers}
          onCreate={onCreateTeam}
        />
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {nav.map((n) => (
                <SidebarMenuItem key={n.to}>
                  <SidebarMenuButton asChild isActive={active(n.to, n.end)}>
                    <NavLink to={n.to} end={n.end}>
                      <n.icon />
                      <span>{n.label}</span>
                    </NavLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <UserMenu me={me} logout={logout} />
      </SidebarFooter>
    </Sidebar>
  )
}
