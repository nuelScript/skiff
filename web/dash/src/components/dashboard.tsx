import { useEffect, useState } from 'react'
import { useApps } from '@/hooks/use-apps'
import { useConsole } from '@/hooks/use-console'
import { useDeploys } from '@/hooks/use-deploys'
import { api, type Me } from '@/services/api.service'
import Header from '@/components/header'
import AppCard from '@/components/app-card'
import DeployModal from '@/components/deploy-modal'
import DeployHistory from '@/components/deploy-history'
import EnvDialog from '@/components/env-dialog'
import MembersDialog from '@/components/members-dialog'
import Drawer from '@/components/drawer'
import { LogoMark } from '@/components/logo'

export default function Dashboard({
  me,
  logout,
  switchTeam,
}: {
  me: Me
  logout: () => void
  switchTeam: (id: string) => Promise<void>
}) {
  const { apps, reload, stop } = useApps()
  const term = useConsole(reload)
  const history = useDeploys()
  const [open, setOpen] = useState(false)
  const [members, setMembers] = useState(false)
  const [envApp, setEnvApp] = useState<string | null>(null)

  useEffect(() => {
    reload()
  }, [me.team, reload])

  const createTeam = async () => {
    const name = window.prompt('New team name')
    if (name && name.trim()) {
      const t = await api.auth.createTeam(name.trim())
      await switchTeam(t.id)
    }
  }

  return (
    <>
      <Header
        me={me}
        onDeploy={() => setOpen(true)}
        onLogout={logout}
        switchTeam={switchTeam}
        onMembers={() => setMembers(true)}
        onCreateTeam={createTeam}
      />
      <main className="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-4 p-6 pb-[45vh]">
        {apps.length === 0 ? (
          <div className="col-span-full flex flex-col items-center gap-3 p-16 text-center">
            <LogoMark className="h-10 w-10 opacity-40" />
            <p className="text-muted-foreground text-sm">
              No apps yet — deploy your first from a git repo.
            </p>
          </div>
        ) : (
          apps.map((a) => (
            <AppCard
              key={a.name}
              app={a}
              onLogs={term.showLogs}
              onHistory={history.open}
              onEnv={setEnvApp}
              onStop={stop}
            />
          ))
        )}
      </main>

      <DeployModal
        open={open}
        onOpenChange={setOpen}
        onDeployUrl={(git, name, port, token) => {
          setOpen(false)
          term.deploy(git, name, port, token)
        }}
        onDeployRepo={(repo, clone, branch, name, port, auto, rootDir) => {
          setOpen(false)
          term.deployRepo(repo, clone, branch, name, port, auto, rootDir)
        }}
      />

      <DeployHistory
        app={history.app}
        deploys={history.deploys}
        onClose={history.close}
        onViewLog={(app, id) => {
          history.close()
          term.showBuildLog(app, id)
        }}
      />

      <MembersDialog open={members} onOpenChange={setMembers} />

      <EnvDialog app={envApp} onClose={() => setEnvApp(null)} />

      {term.stream && <Drawer stream={term.stream} onClose={term.close} />}
    </>
  )
}
