import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { EnvEditor } from '@/components/env-editor'

export function EnvDialog({ app, onClose }: { app: string | null; onClose: () => void }) {
  return (
    <Dialog open={!!app} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            Environment · <span className="font-mono">{app}</span>
          </DialogTitle>
        </DialogHeader>
        {app && <EnvEditor app={app} />}
      </DialogContent>
    </Dialog>
  )
}
