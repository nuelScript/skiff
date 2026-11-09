import { useState } from 'react'
import { Check, Copy, Eye, EyeOff } from 'lucide-react'
import { useCopy } from '@/hooks/use-copy'

// JobWebhook shows where to send a job's webhook and the secret to sign it with.
export function JobWebhook({ url, secret, after }: { url: string; secret: string; after?: React.ReactNode }) {
  const [shown, setShown] = useState(false)
  const full = url.startsWith('/') ? window.location.origin + url : url
  return (
    <div className="mt-3 grid gap-2 rounded-lg border border-foreground/8 p-3 text-xs">
      <Row label="Webhook" value={full} />
      <Row
        label="Secret"
        value={secret}
        display={shown ? secret : '•'.repeat(24)}
        extra={
          <button
            type="button"
            onClick={() => setShown(!shown)}
            aria-label={shown ? 'Hide the secret' : 'Show the secret'}
            className="text-muted-foreground hover:text-foreground cursor-pointer p-1"
          >
            {shown ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
          </button>
        }
      />
      <p className="text-muted-foreground leading-relaxed">
        POST to it with the body signed by HMAC-SHA256 of this secret, sent as{' '}
        <code className="font-mono">X-Skiff-Signature: sha256=…</code>. A GitHub webhook with this secret works
        as it is.{' '}
        {after ?? (
          <>
            The run gets the body in <code className="font-mono">SKIFF_WEBHOOK_BODY</code>.
          </>
        )}
      </p>
    </div>
  )
}

function Row({ label, value, display, extra }: { label: string; value: string; display?: string; extra?: React.ReactNode }) {
  const { copied, copy } = useCopy()
  return (
    <div className="flex min-w-0 items-center gap-2">
      <span className="text-muted-foreground w-16 shrink-0">{label}</span>
      <code className="min-w-0 flex-1 truncate font-mono">{display ?? value}</code>
      {extra}
      <button
        type="button"
        onClick={() => copy(value)}
        aria-label={`Copy the ${label.toLowerCase()}`}
        className="text-muted-foreground hover:text-foreground cursor-pointer p-1"
      >
        {copied ? <Check className="text-success h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
      </button>
    </div>
  )
}
