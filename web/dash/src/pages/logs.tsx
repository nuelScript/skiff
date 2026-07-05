import { ScrollText } from 'lucide-react'
import PagePlaceholder from '@/components/page-placeholder'

export default function LogsPage() {
  return (
    <PagePlaceholder
      icon={ScrollText}
      title="Logs"
      note="Live runtime logs for any app, streamed from the box. Coming soon."
    />
  )
}
