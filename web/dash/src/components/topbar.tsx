import { useLocation } from 'react-router'
import { SidebarTrigger } from '@/components/ui/sidebar'

const titles: Record<string, string> = {
  '/': 'Projects',
  '/deployments': 'Deployments',
  '/logs': 'Logs',
  '/server': 'Server',
  '/domains': 'Domains',
  '/env': 'Environment',
  '/settings': 'Settings',
}

export default function Topbar() {
  const { pathname } = useLocation()
  const title = titles[pathname] ?? 'Skiff'
  return (
    <header className="border-border bg-background/70 sticky top-0 z-30 flex h-14 shrink-0 items-center gap-2 border-b px-4 backdrop-blur-xl">
      <SidebarTrigger className="text-muted-foreground -ml-1" />
      <span className="text-sm font-medium">{title}</span>
    </header>
  )
}
