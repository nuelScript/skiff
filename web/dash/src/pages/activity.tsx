import type { ComponentType } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Rocket,
  RotateCcw,
  Settings,
  KeyRound,
  Trash2,
  Database,
  Undo2,
  Globe,
  UserPlus,
  UserMinus,
  Bell,
  History,
  GitCommitHorizontal,
} from 'lucide-react'
import { auditService, type AuditEntry } from '@/services/api.service'
import { queryKeys } from '@/constants/query-keys'

function rel(unix: number): string {
  const s = Math.max(0, Math.floor(Date.now() / 1000 - unix))
  if (s < 60) return s + 's ago'
  if (s < 3600) return Math.floor(s / 60) + 'm ago'
  if (s < 86400) return Math.floor(s / 3600) + 'h ago'
  return Math.floor(s / 86400) + 'd ago'
}

type Kind = { icon: ComponentType<{ className?: string }>; verb: string; tone: string }

const KINDS: Record<string, Kind> = {
  deploy: { icon: Rocket, verb: 'deployed', tone: 'text-emerald-400' },
  rollback: { icon: RotateCcw, verb: 'rolled back', tone: 'text-amber-400' },
  'settings.update': { icon: Settings, verb: 'updated settings for', tone: 'text-muted-foreground' },
  'env.update': { icon: KeyRound, verb: 'changed environment for', tone: 'text-violet-400' },
  'project.delete': { icon: Trash2, verb: 'deleted project', tone: 'text-rose-400' },
  'database.create': { icon: Database, verb: 'created database', tone: 'text-emerald-400' },
  'database.delete': { icon: Database, verb: 'deleted database', tone: 'text-rose-400' },
  'backup.restore': { icon: Undo2, verb: 'restored a backup of', tone: 'text-amber-400' },
  'domain.add': { icon: Globe, verb: 'added domain', tone: 'text-emerald-400' },
  'domain.remove': { icon: Globe, verb: 'removed domain', tone: 'text-rose-400' },
  'member.invite': { icon: UserPlus, verb: 'invited', tone: 'text-emerald-400' },
  'member.remove': { icon: UserMinus, verb: 'removed', tone: 'text-rose-400' },
  'alerts.update': { icon: Bell, verb: 'updated alert channels', tone: 'text-muted-foreground' },
}

export default function ActivityPage() {
  const { data: entries } = useQuery({
    queryKey: queryKeys.audit,
    queryFn: () => auditService.list(),
    refetchInterval: 15000,
  })

  return (
    <div className="px-8 py-8">
      <header className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight">Activity</h1>
        <p className="text-muted-foreground mt-1 text-sm">
          Every deploy, change, and teardown in this team — who did what, and when.
        </p>
      </header>

      {!entries ? (
        <p className="text-muted-foreground text-sm">Loading activity…</p>
      ) : entries.length === 0 ? (
        <div className="text-muted-foreground flex flex-col items-center gap-2 rounded-xl border border-white/8 py-20 text-sm">
          <History className="h-6 w-6 opacity-40" />
          <span>No activity yet — actions across the team will show up here.</span>
        </div>
      ) : (
        <ol className="max-w-2xl overflow-hidden rounded-xl border border-white/8">
          {entries.map((e, i) => (
            <Row key={e.id} e={e} last={i === entries.length - 1} />
          ))}
        </ol>
      )}
    </div>
  )
}

function Row({ e, last }: { e: AuditEntry; last: boolean }) {
  const k = KINDS[e.action] ?? { icon: GitCommitHorizontal, verb: e.action, tone: 'text-muted-foreground' }
  const Icon = k.icon
  return (
    <li className={'flex items-start gap-3 px-4 py-3 ' + (last ? '' : 'border-b border-white/6')}>
      <span className={'mt-0.5 grid h-7 w-7 shrink-0 place-items-center rounded-full bg-white/5 ' + k.tone}>
        <Icon className="h-3.5 w-3.5" />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm">
          <Actor actor={e.actor} /> <span className="text-muted-foreground">{k.verb}</span>{' '}
          {e.target && <span className="font-mono text-[13px]">{e.target}</span>}
        </p>
        {e.detail && <p className="text-muted-foreground mt-0.5 text-xs">{e.detail}</p>}
      </div>
      <time className="text-muted-foreground shrink-0 text-xs tabular-nums">{rel(e.created)}</time>
    </li>
  )
}

function Actor({ actor }: { actor: string }) {
  if (actor === 'push') {
    return (
      <span className="text-muted-foreground rounded bg-white/8 px-1.5 py-0.5 font-mono text-[11px]">
        push
      </span>
    )
  }
  return <span className="font-medium">{actor.includes('@') ? actor.split('@')[0] : actor}</span>
}
