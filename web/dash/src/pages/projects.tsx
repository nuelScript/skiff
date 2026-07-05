import { useEffect, useState } from 'react'
import { useApps } from '@/hooks/use-apps'
import { useConsole } from '@/hooks/use-console'
import { useDeploys } from '@/hooks/use-deploys'
import { useSystem } from '@/hooks/use-system'
import { useAuthContext } from '@/lib/auth-context'
import { Button } from '@/components/ui/button'
import AppCard from '@/components/app-card'
import ControlPlaneCard from '@/components/control-plane-card'
import DeployModal from '@/components/deploy-modal'
import DeployHistory from '@/components/deploy-history'
import EnvDialog from '@/components/env-dialog'
import Drawer from '@/components/drawer'
import { LogoMark } from '@/components/logo'

export default function ProjectsPage() {
  const { me } = useAuthContext()
  const { apps, reload, stop } = useApps()
  const term = useConsole(reload)
  const history = useDeploys()
  const { info: system } = useSystem()
  const [open, setOpen] = useState(false)
  const [envApp, setEnvApp] = useState<string | null>(null)

  const teamName = me?.teams?.find((t) => t.id === me?.team)?.name ?? 'this team'

  useEffect(() => {
    reload()
  }, [me?.team, reload])

  return (
    <div className="p-6">
      <div className="mb-5 flex items-end justify-between gap-4">
        <div>
          <h1 className="text-lg font-medium">Projects</h1>
          <p className="text-muted-foreground text-sm">
            Apps deployed from Git in {teamName}.
          </p>
        </div>
        <Button size="sm" onClick={() => setOpen(true)}>
          Deploy from Git
        </Button>
      </div>

      <div className="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-4 pb-[45vh]">
        {system?.selfDeploy && (
          <ControlPlaneCard
            info={system}
            onHistory={history.open}
            onLogs={(app, id) => {
              history.close()
              term.showBuildLog(app, id)
            }}
          />
        )}
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
      </div>

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

      <EnvDialog app={envApp} onClose={() => setEnvApp(null)} />

      {term.stream && <Drawer stream={term.stream} onClose={term.close} />}
    </div>
  )
}
