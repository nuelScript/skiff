import { Server } from 'lucide-react'
import PagePlaceholder from '@/components/page-placeholder'

export default function ServerPage() {
  return (
    <PagePlaceholder
      icon={Server}
      title="Server"
      note="Your box, laid bare — CPU, memory, disk, uptime, and every running container. The thing a hosted platform hides from you."
    />
  )
}
