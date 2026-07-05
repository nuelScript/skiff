import type { ComponentType } from 'react'

// A tasteful stub for routes that are scaffolded but not yet built out.
export default function PagePlaceholder({
  icon: Icon,
  title,
  note,
}: {
  icon: ComponentType<{ className?: string }>
  title: string
  note: string
}) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 p-16 text-center">
      <Icon className="text-muted-foreground/40 h-9 w-9" />
      <h2 className="text-lg font-medium">{title}</h2>
      <p className="text-muted-foreground max-w-sm text-sm">{note}</p>
    </div>
  )
}
