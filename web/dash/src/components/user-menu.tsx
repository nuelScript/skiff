import { useNavigate } from 'react-router'
import { LogOut, MoreHorizontal, Settings } from 'lucide-react'
import type { Me } from '@/services/api.service'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export default function UserMenu({ me, logout }: { me: Me; logout: () => void }) {
  const navigate = useNavigate()
  const name = me.user?.name || me.user?.email || 'You'
  const email = me.user?.email ?? ''
  const initial = name.charAt(0).toUpperCase()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="hover:bg-sidebar-accent data-[state=open]:bg-sidebar-accent flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left transition-colors outline-none">
        <span className="bg-sidebar-accent text-foreground flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-medium">
          {initial}
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm leading-tight font-medium">{name}</div>
          <div className="text-muted-foreground truncate text-[11px] leading-tight">{email}</div>
        </div>
        <MoreHorizontal className="text-muted-foreground h-4 w-4 shrink-0" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" side="top" className="w-56">
        <DropdownMenuLabel className="text-muted-foreground truncate text-xs font-normal">
          {email}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => navigate('/settings')} className="gap-2">
          <Settings className="h-3.5 w-3.5" />
          Settings
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => void logout()}
          className="gap-2 text-rose-400 focus:text-rose-400"
        >
          <LogOut className="h-3.5 w-3.5" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
