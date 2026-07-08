import { ChevronRight, ExternalLink, GitBranch, RotateCcw, RotateCw } from 'lucide-react'
import { type ReactNode } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { ConsoleTerminal } from '@/components/console-terminal'
import { Drawer } from '@/components/drawer'
import { EnvEditor } from '@/components/env-editor'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { FeedSkeleton } from '@/components/skeletons'
import { ErrorState } from '@/components/error-state'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useConsole } from '@/hooks/use-console'
import { useProject } from '@/hooks/use-project'
import { relTime } from '@/lib/format'
import { useConfirm } from '@/providers/confirm-provider'
import { projectsService } from '@/services/api.service'
import { JobsPanel } from './project/jobs-panel'
import { WorkersPanel } from './project/workers-panel'
import { SettingsForm } from './project/settings-form'
import { PreviewsPanel } from './project/previews-panel'
import { runningPill, deployDot } from './project/status'

export default function ProjectDetailPage() {
  const { name = '' } = useParams()
  const navigate = useNavigate()
  const { project, isPending, reload } = useProject(name)
  const term = useConsole(reload)
  const confirm = useConfirm()

  if (isPending) {
    return (
      <div className="px-8 py-8">
        <Skeleton className="mb-4 h-4 w-40" />
        <div className="mb-6 flex items-center gap-3">
          <Skeleton className="h-11 w-11 rounded-lg" />
          <div className="space-y-2">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-3.5 w-56" />
          </div>
        </div>
        <FeedSkeleton rows={4} />
      </div>
    )
  }

  if (!project) {
    return (
      <div className="px-8 py-8">
        <ErrorState message="Couldn't load this project — it may have been removed." />
      </div>
    )
  }

  const latest = project.deploys[0]
  const host = project.url.replace(/^https?:\/\//, '')

  return (
    <div className="px-8 py-8">
      {/* breadcrumb */}
      <div className="text-muted-foreground mb-4 flex items-center gap-1.5 text-sm">
        <Link to="/" className="hover:text-foreground transition-colors">
          Projects
        </Link>
        <ChevronRight className="h-3.5 w-3.5" />
        <span className="text-foreground font-medium">{project.name}</span>
      </div>

      {/* header */}
      <header className="mb-6 flex flex-wrap items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold tracking-tight">{project.name}</h1>
          <span
            className={
              'inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 font-mono text-[10px] font-medium tracking-wider uppercase ' +
              runningPill(project.state)
            }
          >
            <span
              className={
                'h-1.5 w-1.5 rounded-full ' +
                (project.state === 'running' ? 'bg-emerald-400' : 'bg-rose-400')
              }
            />
            {project.state}
          </span>
        </div>
        <div className="flex items-center gap-2">
          {project.repo && (
            <Button size="sm" variant="outline" asChild>
              <a href={'https://github.com/' + project.repo} target="_blank" rel="noreferrer">
                <GitBranch />
                Repository
              </a>
            </Button>
          )}
          <Button size="sm" variant="outline" onClick={() => term.redeploy(project.name)}>
            <RotateCw />
            Redeploy
          </Button>
          <Button size="sm" asChild>
            <a href={project.url} target="_blank" rel="noreferrer">
              <ExternalLink />
              Visit
            </a>
          </Button>
        </div>
      </header>

      <Tabs defaultValue="overview">
        <TabsList
          variant="line"
          className="mb-6 w-full justify-start gap-6 border-b border-white/8"
        >
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="deployments">Deployments</TabsTrigger>
          <TabsTrigger value="previews">
            Previews
            {project.previews.length > 0 && (
              <span className="text-muted-foreground ml-1.5 rounded-full bg-white/8 px-1.5 py-0.5 font-mono text-[10px] tabular-nums">
                {project.previews.length}
              </span>
            )}
          </TabsTrigger>
          <TabsTrigger value="console">Console</TabsTrigger>
          <TabsTrigger value="jobs">Jobs</TabsTrigger>
          <TabsTrigger value="workers">Workers</TabsTrigger>
          <TabsTrigger value="environment">Environment</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        {/* Overview */}
        <TabsContent value="overview" className="pb-[46vh]">
          <div className="grid gap-4 lg:grid-cols-[1.6fr_1fr]">
            <section className="rounded-xl border border-white/8 bg-linear-to-b from-white/2.5 to-transparent p-5">
              <div className="mb-4 flex items-center justify-between">
                <h2 className="text-muted-foreground font-mono text-[11px] tracking-wider uppercase">
                  Production Deployment
                </h2>
                {latest && (
                  <span
                    className={
                      'font-mono text-[10px] tracking-wider uppercase ' +
                      (latest.status === 'live'
                        ? 'text-emerald-300'
                        : latest.status === 'failed'
                          ? 'text-rose-300'
                          : 'text-amber-300')
                    }
                  >
                    {latest.status === 'live' ? 'Ready' : latest.status}
                  </span>
                )}
              </div>
              {latest ? (
                <div className="flex flex-col gap-4">
                  <a
                    href={project.url}
                    target="_blank"
                    rel="noreferrer"
                    className="group hover:text-foreground flex w-fit items-center gap-2 font-mono text-[15px] font-medium transition-colors"
                  >
                    <span className={'h-2 w-2 shrink-0 rounded-full ' + deployDot(latest.status)} />
                    {host}
                    <ExternalLink className="h-3.5 w-3.5 opacity-0 transition-opacity group-hover:opacity-100" />
                  </a>
                  <div className="flex flex-col gap-1.5 border-t border-white/5 pt-4">
                    <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                      {project.repo ? (
                        <a
                          href={'https://github.com/' + project.repo + '/commit/' + latest.commit}
                          target="_blank"
                          rel="noreferrer"
                          className="font-mono text-sm hover:underline"
                        >
                          {latest.commit || '—'}
                        </a>
                      ) : (
                        <span className="font-mono text-sm">{latest.commit || '—'}</span>
                      )}
                      <span className="text-muted-foreground font-mono text-xs">
                        on {project.branch || 'main'} · {latest.trigger} · {relTime(latest.started)}
                      </span>
                    </div>
                    {latest.message && (
                      <p className="text-foreground/90 text-sm">{latest.message}</p>
                    )}
                  </div>
                </div>
              ) : (
                <p className="text-muted-foreground text-sm">
                  No deployments yet — push to{' '}
                  <span className="font-mono">{project.branch || 'main'}</span> or hit Redeploy.
                </p>
              )}
            </section>

            <section className="rounded-xl border border-white/8 bg-linear-to-b from-white/2.5 to-transparent p-5">
              <h2 className="text-muted-foreground mb-2 font-mono text-[11px] tracking-wider uppercase">
                Source
              </h2>
              <dl className="divide-y divide-white/5">
                <Row label="Repository">
                  {project.repo ? (
                    <a
                      href={'https://github.com/' + project.repo}
                      target="_blank"
                      rel="noreferrer"
                      className="font-mono text-xs hover:underline"
                    >
                      {project.repo}
                    </a>
                  ) : (
                    <span className="font-mono text-xs">—</span>
                  )}
                </Row>
                <Row label="Branch">
                  <span className="font-mono text-xs">{project.branch || 'main'}</span>
                </Row>
                <Row label="Root directory">
                  <span className="font-mono text-xs">{project.rootDir || '/'}</span>
                </Row>
                <Row label="Port">
                  <span className="font-mono text-xs">{project.port}</span>
                </Row>
                <Row label="Auto-deploy">
                  <span
                    className={
                      'font-mono text-xs ' +
                      (project.auto ? 'text-emerald-300' : 'text-muted-foreground')
                    }
                  >
                    {project.auto ? 'On' : 'Off'}
                  </span>
                </Row>
              </dl>
            </section>
          </div>
        </TabsContent>

        {/* Deployments */}
        <TabsContent value="deployments" className="pb-[46vh]">
          <div className="overflow-hidden rounded-xl border border-white/8">
            {project.deploys.length === 0 ? (
              <p className="text-muted-foreground p-8 text-center text-sm">No deployments yet.</p>
            ) : (
              project.deploys.map((d) => (
                <div
                  key={d.id}
                  className="group flex items-center gap-2 border-b border-white/5 pr-3 transition-colors last:border-0 hover:bg-white/[0.03]"
                >
                  <button
                    onClick={() => term.showBuildLog(project.name, d.id)}
                    className="flex min-w-0 flex-1 items-center gap-4 px-5 py-3.5 text-left"
                  >
                    <span className={'h-2 w-2 shrink-0 rounded-full ' + deployDot(d.status)} />
                    <span className="min-w-0 flex-1 truncate text-sm">
                      {d.message || d.commit || d.status}
                    </span>
                    {d.commit && (
                      <span className="text-muted-foreground shrink-0 font-mono text-xs">
                        {d.commit}
                      </span>
                    )}
                    <span className="text-muted-foreground w-16 shrink-0 font-mono text-[11px]">
                      {d.trigger}
                    </span>
                    <span className="text-muted-foreground w-16 shrink-0 text-right font-mono text-[11px]">
                      {relTime(d.started)}
                    </span>
                  </button>
                  {d.rollbackable && (
                    <button
                      onClick={async () => {
                        if (
                          await confirm({
                            title: `Roll back ${project.name}?`,
                            description: 'It re-runs this build instantly — no rebuild.',
                            confirmText: 'Roll back',
                          })
                        )
                          term.rollback(project.name, d.id)
                      }}
                      title="Instant rollback to this build"
                      className="text-muted-foreground hover:text-foreground flex shrink-0 items-center gap-1 rounded-[6px] border border-white/10 px-2 py-1 text-[11px] opacity-0 transition group-hover:opacity-100 hover:border-white/20"
                    >
                      <RotateCcw className="h-3 w-3" />
                      Rollback
                    </button>
                  )}
                </div>
              ))
            )}
          </div>
        </TabsContent>

        {/* Previews */}
        <TabsContent value="previews" className="pb-[46vh]">
          <PreviewsPanel
            project={project.name}
            previews={project.previews}
            onCreate={(branch) => term.preview(project.name, branch)}
            onTeardown={async (name) => {
              await projectsService.stop(name)
              reload()
            }}
          />
        </TabsContent>

        {/* Console */}
        <TabsContent value="console" className="pb-8">
          <div className="mb-3">
            <h2 className="text-sm font-semibold">Console</h2>
            <p className="text-muted-foreground mt-0.5 text-xs">
              A shell inside the running container — run migrations, inspect state, debug.
            </p>
          </div>
          {project.state === 'running' ? (
            <div className="h-[60vh] overflow-hidden rounded-xl border border-white/10 bg-[#09090b] p-3">
              <ConsoleTerminal app={project.name} />
            </div>
          ) : (
            <p className="text-muted-foreground rounded-xl border border-white/8 py-16 text-center text-sm">
              The app isn't running — deploy it to open a console.
            </p>
          )}
        </TabsContent>

        {/* Environment */}
        <TabsContent value="environment" className="max-w-2xl pb-[46vh]">
          <EnvEditor app={project.name} />
        </TabsContent>

        {/* Jobs */}
        <TabsContent value="jobs" className="max-w-3xl pb-[46vh]">
          <JobsPanel app={project.name} />
        </TabsContent>

        <TabsContent value="workers" className="max-w-3xl pb-[46vh]">
          <WorkersPanel app={project.name} />
        </TabsContent>

        {/* Settings */}
        <TabsContent value="settings" className="max-w-2xl pb-[46vh]">
          <SettingsForm
            name={project.name}
            branch={project.branch}
            rootDir={project.rootDir}
            port={project.port}
            auto={project.auto}
            previewAuto={project.previewAuto}
            replicas={project.replicas}
            running={project.running}
            release={project.release}
            autoscale={project.autoscale}
            scaleMin={project.scaleMin}
            scaleMax={project.scaleMax}
            scaleCpu={project.scaleCpu}
            onSaved={reload}
            onDeleted={() => navigate('/')}
            onRedeploy={() => term.redeploy(project.name)}
          />
        </TabsContent>
      </Tabs>

      {term.stream && <Drawer stream={term.stream} onClose={term.close} onStop={term.stop} />}
    </div>
  )
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 py-2.5 text-sm first:pt-0 last:pb-0">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="truncate text-right">{children}</dd>
    </div>
  )
}
