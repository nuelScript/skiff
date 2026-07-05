import { Button } from '@/components/ui/button'
import { Logo } from '@/components/logo'
import TeamSwitcher from '@/components/team-switcher'
import type { Me } from '@/services/api.service'

export default function Header({
  me,
  onDeploy,
  onLogout,
  switchTeam,
  onMembers,
  onCreateTeam,
}: {
  me: Me
  onDeploy: () => void
  onLogout: () => void
  switchTeam: (id: string) => Promise<void>
  onMembers: () => void
  onCreateTeam: () => void
}) {
  return (
    <header className="border-border bg-background/70 sticky top-0 z-40 flex items-center gap-3 border-b px-6 py-3.5 backdrop-blur-xl">
      <Logo />
      <span className="text-border">/</span>
      <TeamSwitcher
        me={me}
        switchTeam={switchTeam}
        onMembers={onMembers}
        onCreate={onCreateTeam}
      />
      <div className="flex-1" />
      <span className="text-muted-foreground hidden text-sm sm:inline">
        {me.user?.email}
      </span>
      <Button onClick={onDeploy} size="sm">
        Deploy from Git
      </Button>
      <Button onClick={onLogout} variant="ghost" size="sm">
        Sign out
      </Button>
    </header>
  )
}
