import { Globe } from 'lucide-react'
import PagePlaceholder from '@/components/page-placeholder'

export default function DomainsPage() {
  return (
    <PagePlaceholder
      icon={Globe}
      title="Domains"
      note="Every app's URL in one place, and custom domains with automatic certificates. Coming soon."
    />
  )
}
