import { BrowserRouter, Routes, Route, Navigate, Outlet, useParams } from 'react-router'
import { AuthProvider, useAuthContext } from '@/lib/auth-context'
import Setup from '@/components/setup'
import Login from '@/components/login'
import AcceptInvite from '@/components/accept-invite'
import Shell from '@/components/shell'
import ProjectsPage from '@/pages/projects'
import DeploymentsPage from '@/pages/deployments'
import LogsPage from '@/pages/logs'
import ServerPage from '@/pages/server'
import DomainsPage from '@/pages/domains'
import EnvPage from '@/pages/env'
import SettingsPage from '@/pages/settings'

function InviteRoute() {
  const { token } = useParams()
  return <AcceptInvite token={token ?? ''} />
}

// AuthGate short-circuits the whole shell until the visitor is signed in.
function AuthGate() {
  const auth = useAuthContext()
  if (auth.state === 'checking')
    return (
      <div className="text-muted-foreground flex h-screen items-center justify-center">
        …
      </div>
    )
  if (auth.state === 'setup') return <Setup onSetup={auth.setup} />
  if (auth.state === 'out') return <Login onLogin={auth.login} />
  return <Outlet />
}

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/invite/:token" element={<InviteRoute />} />
          <Route element={<AuthGate />}>
            <Route element={<Shell />}>
              <Route index element={<ProjectsPage />} />
              <Route path="deployments" element={<DeploymentsPage />} />
              <Route path="logs" element={<LogsPage />} />
              <Route path="server" element={<ServerPage />} />
              <Route path="domains" element={<DomainsPage />} />
              <Route path="env" element={<EnvPage />} />
              <Route path="settings" element={<SettingsPage />} />
            </Route>
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
