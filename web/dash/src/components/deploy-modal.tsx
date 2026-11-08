import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import {
  AlertTriangle,
  ArrowLeft,
  Box,
  Database,
  GitBranch,
  LayoutGrid,
  Link2,
  MemoryStick,
  Search,
  Plus,
} from 'lucide-react'
import { useGithub } from '@/hooks/use-github'
import { useAuthContext } from '@/context/auth-context'
import { useDeployForm } from '@/hooks/use-deploy-form'
import {
  envService,
  projectsService,
  type CatalogApp,
  type EnvVar,
  type Repo,
} from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { EnvFields, blankVar } from '@/components/env-fields'
import { errText } from '@/lib/errors'
import { imageProjectSchema, type ImageProjectInput } from '@/validations/image'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onDeployUrl: (git: string, name: string, port: string, token: string) => void
  onDeployRepo: (
    repo: string,
    clone: string,
    branch: string,
    name: string,
    port: string,
    auto: boolean,
    rootDir: string,
  ) => void
  onDeployImage: (app: string, id: string) => void
}

type Mode = 'github' | 'url' | 'image' | 'apps'

const descriptions: Record<Mode, string> = {
  github: 'Deploy from GitHub and auto-deploy on every push.',
  url: 'Clone a repo by URL and run it on your server.',
  image: 'Run an image that is already built, from Docker Hub or any registry.',
  apps: 'Install a ready-made app, with everything it needs set up for you.',
}

const tabs: { mode: Mode; label: string; short: string; icon: typeof Box }[] = [
  { mode: 'github', label: 'GitHub', short: 'GitHub', icon: GitBranch },
  { mode: 'apps', label: 'One-click apps', short: 'Apps', icon: LayoutGrid },
  { mode: 'image', label: 'Docker image', short: 'Image', icon: Box },
  { mode: 'url', label: 'Git URL', short: 'Git URL', icon: Link2 },
]

export function DeployModal({ open, onOpenChange, onDeployUrl, onDeployRepo, onDeployImage }: Props) {
  const gh = useGithub(open)
  const { me } = useAuthContext()
  const [mode, setMode] = useState<Mode>('github')
  const [selected, setSelected] = useState<Repo | null>(null)
  const [search, setSearch] = useState('')
  const [name, setName] = useState('')
  const [port, setPort] = useState('3000')
  const [rootDir, setRootDir] = useState('')
  const [auto, setAuto] = useState(true)
  const [envVars, setEnvVars] = useState<EnvVar[]>([blankVar()])
  const [showEnv, setShowEnv] = useState(false)

  useEffect(() => {
    if (!open) {
      setSelected(null)
      setSearch('')
      setMode('github')
    }
  }, [open])

  const filtered = useMemo(
    () =>
      gh.repos.filter((r) => r.full_name.toLowerCase().includes(search.toLowerCase())).slice(0, 40),
    [gh.repos, search],
  )

  const pick = (r: Repo) => {
    setSelected(r)
    setName(r.name)
    setPort('3000')
    setRootDir('')
    setAuto(true)
    setEnvVars([blankVar()])
    setShowEnv(false)
  }

  const deployRepo = async () => {
    if (!selected || !name.trim()) return
    const filledEnv = envVars.filter((v) => v.key.trim())
    if (filledEnv.length) {
      try {
        await envService.save(name.trim(), filledEnv)
      } catch {
        /* best effort — env can still be set after deploy */
      }
    }
    onDeployRepo(
      selected.full_name,
      selected.clone_url,
      selected.default_branch,
      name.trim(),
      port.trim() || '3000',
      auto,
      rootDir.trim(),
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Deploy an app</DialogTitle>
          <DialogDescription>{descriptions[mode]}</DialogDescription>
        </DialogHeader>

        <Tabs value={mode} onValueChange={(v) => setMode(v as Mode)}>
          <TabsList className="w-full">
            {tabs.map((t) => (
              <TabsTrigger key={t.mode} value={t.mode} className="cursor-pointer">
                <t.icon className="max-sm:hidden" />
                <span className="sm:hidden">{t.short}</span>
                <span className="max-sm:hidden">{t.label}</span>
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        {mode === 'apps' ? (
          <CatalogPicker onDeployed={onDeployImage} />
        ) : mode === 'image' ? (
          <ImageForm onDeployed={onDeployImage} />
        ) : mode === 'url' ? (
          <UrlForm onDeploy={onDeployUrl} />
        ) : gh.status === null ? (
          <p className="text-muted-foreground py-8 text-center text-sm">…</p>
        ) : !gh.status.configured ? (
          <ConnectCard
            hint={
              me?.admin
                ? 'Create the Skiff GitHub App once to deploy any of your repositories.'
                : "GitHub isn't connected yet. Ask whoever set up this Skiff to connect it."
            }
            label="Connect GitHub"
            href={me?.admin ? '/api/github/create' : undefined}
          />
        ) : gh.status.usable === false ? (
          <ConnectCard
            hint="This server's GitHub connection belongs to the team that set it up. Deploy from a git URL with your own token instead."
            label="Not available to your team"
          />
        ) : !gh.status.installed ? (
          <ConnectCard
            hint="Choose which repositories Skiff can deploy."
            label="Install on repositories"
            href={`https://github.com/apps/${gh.status.slug}/installations/new`}
          />
        ) : selected ? (
          <div className="flex flex-col gap-4">
            <button
              onClick={() => setSelected(null)}
              className="text-muted-foreground hover:text-foreground text-left font-mono text-xs"
            >
              ← {selected.full_name}
            </button>
            <div className="grid gap-2">
              <Label htmlFor="rn">App name</Label>
              <Input id="rn" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="rp">Port your app listens on</Label>
              <Input id="rp" value={port} onChange={(e) => setPort(e.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="rd">
                Root directory{' '}
                <span className="text-muted-foreground">— for monorepos (optional)</span>
              </Label>
              <Input
                id="rd"
                placeholder="e.g. apps/web"
                value={rootDir}
                onChange={(e) => setRootDir(e.target.value)}
              />
            </div>

            {showEnv ? (
              <div className="grid gap-2">
                <Label>Environment variables</Label>
                <EnvFields vars={envVars} onChange={setEnvVars} />
              </div>
            ) : (
              <button
                type="button"
                onClick={() => setShowEnv(true)}
                className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1.5 text-sm transition-colors"
              >
                <Plus className="h-3.5 w-3.5" /> Add environment variables
              </button>
            )}

            <button
              type="button"
              onClick={() => setAuto(!auto)}
              className="flex items-center justify-between rounded-md border px-3 py-2.5 text-sm"
            >
              <span>Auto-deploy on every push</span>
              <span
                className={
                  'font-mono text-[11px] uppercase ' +
                  (auto ? 'text-success' : 'text-muted-foreground')
                }
              >
                {auto ? 'on' : 'off'}
              </span>
            </button>
            <Button onClick={deployRepo}>Deploy {selected.name}</Button>
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            <div className="relative">
              <Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
              <Input
                placeholder="Search repositories…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9"
              />
            </div>
            <div className="overflow-hidden rounded-md border border-foreground/8">
              <div className="max-h-64 divide-y divide-foreground/5 overflow-auto">
                {gh.loadingRepos ? (
                  <p className="text-muted-foreground p-4 text-center text-sm">
                    Loading repositories…
                  </p>
                ) : filtered.length === 0 ? (
                  <p className="text-muted-foreground p-4 text-center text-sm">No repositories.</p>
                ) : (
                  filtered.map((r) => (
                    <button
                      key={r.full_name}
                      onClick={() => pick(r)}
                      className="flex w-full items-center justify-between gap-2 px-3 py-2.5 text-left transition-colors hover:bg-foreground/[0.04]"
                    >
                      <span className="min-w-0 truncate font-mono text-sm">{r.full_name}</span>
                      {r.private && (
                        <span className="text-muted-foreground shrink-0 font-mono text-[10px] uppercase">
                          private
                        </span>
                      )}
                    </button>
                  ))
                )}
              </div>
            </div>
          </div>
        )}

      </DialogContent>
    </Dialog>
  )
}

function ConnectCard({ hint, label, href }: { hint: string; label: string; href?: string }) {
  return (
    <div className="flex flex-col items-center gap-4 py-8 text-center">
      <GitBranch className="text-muted-foreground h-8 w-8" />
      <p className="text-muted-foreground max-w-xs text-sm">{hint}</p>
      {href && (
        <Button asChild>
          <a href={href}>{label}</a>
        </Button>
      )}
    </div>
  )
}

function UrlForm({
  onDeploy,
}: {
  onDeploy: (git: string, name: string, port: string, token: string) => void
}) {
  const f = useDeployForm(onDeploy)
  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-2">
        <Label htmlFor="git">Git repository URL</Label>
        <Input
          id="git"
          autoFocus
          placeholder="https://github.com/you/app.git"
          value={f.git}
          onChange={(e) => f.setGit(e.target.value)}
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="n">App name</Label>
        <Input
          id="n"
          placeholder="my-app"
          value={f.name}
          onChange={(e) => f.setName(e.target.value)}
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="p">Port your app listens on</Label>
        <Input id="p" value={f.port} onChange={(e) => f.setPort(e.target.value)} />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="t">
          Access token <span className="text-muted-foreground">— private repos only</span>
        </Label>
        <Input
          id="t"
          type="password"
          placeholder="ghp_… (optional)"
          value={f.token}
          onChange={(e) => f.setToken(e.target.value)}
        />
      </div>
      <Button onClick={f.submit}>Deploy</Button>
    </div>
  )
}

function ImageForm({ onDeployed }: { onDeployed: (app: string, id: string) => void }) {
  const [envVars, setEnvVars] = useState<EnvVar[]>([blankVar()])
  const [showEnv, setShowEnv] = useState(false)
  const [showAuth, setShowAuth] = useState(false)
  const [serverError, setServerError] = useState('')
  const form = useForm<ImageProjectInput>({
    resolver: zodResolver(imageProjectSchema),
    defaultValues: { name: '', image: '', port: '', username: '', password: '' },
  })
  const errors = form.formState.errors

  const submit = form.handleSubmit(async (values) => {
    setServerError('')
    const filledEnv = envVars.filter((v) => v.key.trim())
    if (filledEnv.length) {
      try {
        await envService.save(values.name, filledEnv)
      } catch {
        /* best effort — env can still be set after deploy */
      }
    }
    try {
      const { app, id } = await projectsService.createFromImage(values)
      onDeployed(app, id)
    } catch (err) {
      setServerError(errText(err, 'Could not deploy the image.'))
    }
  })

  // Suggest an app name from the image's repository, e.g. ghcr.io/you/api:1 → api.
  const suggestName = (ref: string) => {
    if (form.getValues('name')) return
    const repo = ref.split('@')[0].split('/').pop()?.split(':')[0] ?? ''
    const name = repo.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/^-+/, '')
    if (name) form.setValue('name', name, { shouldValidate: form.formState.isSubmitted })
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <FieldGroup className="gap-4">
        <Field data-invalid={!!errors.image}>
          <FieldLabel htmlFor="img">Image</FieldLabel>
          <Input
            id="img"
            autoFocus
            className="font-mono"
            placeholder="ghcr.io/you/app:latest"
            aria-invalid={!!errors.image}
            {...form.register('image', { onBlur: (e) => suggestName(e.target.value.trim()) })}
          />
          <FieldError errors={[errors.image]} />
        </Field>
        <Field data-invalid={!!errors.name}>
          <FieldLabel htmlFor="img-name">App name</FieldLabel>
          <Input
            id="img-name"
            placeholder="my-app"
            aria-invalid={!!errors.name}
            {...form.register('name')}
          />
          <FieldError errors={[errors.name]} />
        </Field>
        <Field data-invalid={!!errors.port}>
          <FieldLabel htmlFor="img-port">Port your app listens on</FieldLabel>
          <Input
            id="img-port"
            placeholder="From the image"
            aria-invalid={!!errors.port}
            {...form.register('port')}
          />
          <FieldDescription>Leave empty to use the port the image exposes.</FieldDescription>
          <FieldError errors={[errors.port]} />
        </Field>

        {showAuth ? (
          <div className="grid gap-3 sm:grid-cols-2">
            <Field data-invalid={!!errors.username}>
              <FieldLabel htmlFor="img-user">Registry username</FieldLabel>
              <Input id="img-user" autoComplete="off" {...form.register('username')} />
            </Field>
            <Field data-invalid={!!errors.password}>
              <FieldLabel htmlFor="img-pass">Password or token</FieldLabel>
              <Input
                id="img-pass"
                type="password"
                autoComplete="new-password"
                aria-invalid={!!errors.password}
                {...form.register('password')}
              />
              <FieldError errors={[errors.password]} />
            </Field>
          </div>
        ) : (
          <button
            type="button"
            onClick={() => setShowAuth(true)}
            className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1.5 text-sm transition-colors"
          >
            <Plus className="h-3.5 w-3.5" /> Private image? Add registry credentials
          </button>
        )}

        {showEnv ? (
          <div className="grid gap-2">
            <Label>Environment variables</Label>
            <EnvFields vars={envVars} onChange={setEnvVars} />
          </div>
        ) : (
          <button
            type="button"
            onClick={() => setShowEnv(true)}
            className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1.5 text-sm transition-colors"
          >
            <Plus className="h-3.5 w-3.5" /> Add environment variables
          </button>
        )}
      </FieldGroup>
      {serverError && <p className="text-xs text-danger">{serverError}</p>}
      <Button type="submit" loading={form.formState.isSubmitting}>
        Deploy
      </Button>
    </form>
  )
}

const DB_LABEL: Record<string, string> = { postgres: 'Postgres', mysql: 'MySQL', mongodb: 'MongoDB', redis: 'Redis' }

function CatalogPicker({ onDeployed }: { onDeployed: (app: string, id: string) => void }) {
  const { data: apps = [], isPending } = useQuery({
    queryKey: queryKeys.catalog,
    queryFn: () => projectsService.catalog(),
  })
  const [picked, setPicked] = useState<CatalogApp | null>(null)
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState('')

  const categories = useMemo(() => [...new Set(apps.map((a) => a.category))].sort(), [apps])
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase()
    return apps.filter(
      (a) =>
        (!category || a.category === category) &&
        (!q || [a.name, a.blurb, a.category].some((s) => s.toLowerCase().includes(q))),
    )
  }, [apps, query, category])

  if (isPending) return <p className="text-muted-foreground py-8 text-center text-sm">…</p>

  if (!picked) {
    return (
      <div className="flex min-w-0 flex-col gap-3">
        <div className="relative">
          <Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
          <Input
            type="search"
            aria-label="Search apps"
            placeholder={`Search ${apps.length} apps…`}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="pl-9"
          />
        </div>
        <div className="-mx-1 flex gap-1.5 overflow-x-auto px-1 pb-1 sm:flex-wrap sm:overflow-visible sm:pb-0" role="group" aria-label="Category">
          {['', ...categories].map((c) => (
            <button
              key={c || 'all'}
              type="button"
              aria-pressed={category === c}
              onClick={() => setCategory(c)}
              className={
                'shrink-0 cursor-pointer rounded-full border px-3 py-1 text-xs transition-colors ' +
                (category === c
                  ? 'border-foreground/25 bg-foreground/[0.08] text-foreground'
                  : 'text-muted-foreground hover:text-foreground border-foreground/10')
              }
            >
              {c || 'All'}
            </button>
          ))}
        </div>
        {shown.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-8 text-center text-sm sm:h-[min(52vh,30rem)]">
            <p className="text-muted-foreground">No apps match that.</p>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setQuery('')
                setCategory('')
              }}
            >
              Show all apps
            </Button>
          </div>
        ) : (
          <div className="grid max-h-[min(52vh,30rem)] content-start gap-2 overflow-y-auto sm:h-[min(52vh,30rem)] sm:grid-cols-2">
            {shown.map((a) => (
              <button
                key={a.id}
                type="button"
                onClick={() => {
                  setPicked(a)
                  setName(a.id)
                  setError('')
                }}
                className="hover:border-foreground/25 flex cursor-pointer flex-col gap-1 rounded-lg border border-foreground/10 p-3 text-left transition-colors"
              >
                <span className="flex items-center justify-between gap-2">
                  <span className="text-sm font-semibold">{a.name}</span>
                  <span className="text-muted-foreground text-[11px]">{a.category}</span>
                </span>
                <span className="text-muted-foreground text-xs leading-relaxed">{a.blurb}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    )
  }

  const install = async () => {
    setBusy(true)
    setError('')
    try {
      const { app, id } = await projectsService.install(picked.id, name.trim())
      onDeployed(app, id)
    } catch (err) {
      setError(errText(err, 'Could not install it.'))
      setBusy(false)
    }
  }
  const validName = /^[a-z0-9][a-z0-9-]*$/.test(name.trim())

  return (
    <div className="flex flex-col gap-4">
      <button
        type="button"
        onClick={() => setPicked(null)}
        className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1.5 text-xs"
      >
        <ArrowLeft className="h-3.5 w-3.5" /> All apps
      </button>
      <div>
        <h3 className="text-base font-semibold">{picked.name}</h3>
        <p className="text-muted-foreground mt-1 text-sm">{picked.blurb}</p>
      </div>
      <div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs">
        <span className="flex items-center gap-1.5">
          <MemoryStick className="h-3.5 w-3.5" /> Needs about {picked.memory}
        </span>
        {picked.database && (
          <span className="flex items-center gap-1.5">
            <Database className="h-3.5 w-3.5" /> Includes a {DB_LABEL[picked.database] ?? picked.database} database
          </span>
        )}
      </div>
      <div className="grid gap-2">
        <Label htmlFor="cat-name">App name</Label>
        <Input
          id="cat-name"
          value={name}
          aria-invalid={!validName}
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      <p className="flex gap-2 rounded-lg border border-warning/30 bg-warning/5 p-3 text-xs leading-relaxed">
        <AlertTriangle className="text-warning mt-0.5 h-3.5 w-3.5 shrink-0" />
        <span>
          <strong className="font-semibold">Do this right after installing.</strong> {picked.setup}
        </span>
      </p>
      {error && <p className="text-danger text-xs">{error}</p>}
      <Button onClick={install} loading={busy} disabled={!validName}>
        Install {picked.name}
      </Button>
    </div>
  )
}
