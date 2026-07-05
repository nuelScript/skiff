import { Rocket } from 'lucide-react'
import PagePlaceholder from '@/components/page-placeholder'

export default function DeploymentsPage() {
  return (
    <PagePlaceholder
      icon={Rocket}
      title="Deployments"
      note="A unified build history across every project, with filters and searchable logs. Coming next."
    />
  )
}
