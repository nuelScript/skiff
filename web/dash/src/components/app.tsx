import { useAuth } from '@/hooks/use-auth'
import Setup from '@/components/setup'
import Login from '@/components/login'
import AcceptInvite from '@/components/accept-invite'
import Dashboard from '@/components/dashboard'

export default function App() {
  const auth = useAuth()

  const path = window.location.pathname
  if (path.startsWith('/invite/')) {
    return <AcceptInvite token={path.slice('/invite/'.length)} />
  }

  if (auth.state === 'checking')
    return (
      <div className="text-muted-foreground flex h-screen items-center justify-center">
        …
      </div>
    )
  if (auth.state === 'setup') return <Setup onSetup={auth.setup} />
  if (auth.state === 'out') return <Login onLogin={auth.login} />
  return (
    <Dashboard
      me={auth.me!}
      logout={auth.logout}
      switchTeam={auth.switchTeam}
    />
  )
}
