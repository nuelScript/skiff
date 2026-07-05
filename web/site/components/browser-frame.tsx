import type { ReactNode } from "react";

export function BrowserFrame({
  url,
  right,
  className = "",
  children,
}: {
  url: string;
  right?: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      className={
        "border-line bg-surface/90 overflow-hidden rounded-2xl border backdrop-blur " +
        className
      }
    >
      <div className="border-line flex items-center gap-3 border-b px-4 py-3">
        <div className="flex gap-1.5">
          <span className="bg-line-strong h-2.5 w-2.5 rounded-full" />
          <span className="bg-line-strong h-2.5 w-2.5 rounded-full" />
          <span className="bg-line-strong h-2.5 w-2.5 rounded-full" />
        </div>
        <div className="border-line bg-bg/60 text-muted mx-auto flex items-center gap-2 rounded-md border px-3 py-1 font-mono text-[11px]">
          <span className="bg-signal h-1.5 w-1.5 rounded-full shadow-[0_0_8px_var(--color-signal)]" />
          {url}
        </div>
        {right ? (
          <span className="text-subtle font-mono text-[11px]">{right}</span>
        ) : (
          <span className="w-8" />
        )}
      </div>
      {children}
    </div>
  );
}
