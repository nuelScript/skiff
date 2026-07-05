import { KeyRound } from 'lucide-react'
import PagePlaceholder from '@/components/page-placeholder'

export default function EnvPage() {
  return (
    <PagePlaceholder
      icon={KeyRound}
      title="Environment"
      note="Manage environment variables and secrets across your projects in one place. Coming soon."
    />
  )
}
