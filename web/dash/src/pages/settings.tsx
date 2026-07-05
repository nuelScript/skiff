import { Settings } from 'lucide-react'
import PagePlaceholder from '@/components/page-placeholder'

export default function SettingsPage() {
  return (
    <PagePlaceholder
      icon={Settings}
      title="Settings"
      note="Team, members, GitHub connection, and instance configuration. Coming soon."
    />
  )
}
