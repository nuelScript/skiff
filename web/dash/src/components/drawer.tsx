import { useAutoScroll } from '@/hooks/use-auto-scroll'
import type { Stream } from '@/hooks/use-console'
import { Button } from '@/components/ui/button'

export default function Drawer({
  stream,
  onClose,
}: {
  stream: Stream
  onClose: () => void
}) {
  const ref = useAutoScroll<HTMLPreElement>(stream.lines.length)

  return (
    <div className="bg-popover fixed inset-x-0 bottom-0 flex h-[42vh] flex-col border-t">
      <div className="flex items-center justify-between border-b px-4 py-2.5">
        <span className="text-foreground flex items-center gap-2 font-mono text-xs">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
          {stream.title}
        </span>
        <Button size="sm" variant="ghost" onClick={onClose}>
          Close
        </Button>
      </div>
      <pre
        ref={ref}
        className="text-muted-foreground m-0 flex-1 overflow-auto px-4 py-3 font-mono text-xs whitespace-pre-wrap"
      >
        {stream.lines.join('\n')}
      </pre>
    </div>
  )
}
