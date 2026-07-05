import type { ReactNode } from "react";

export function Callout({ children }: { children: ReactNode }) {
  return (
    <div className="not-prose border-line bg-surface text-muted my-6 rounded-xl border-l-2 border-l-white/40 px-4 py-3 text-sm leading-relaxed">
      {children}
    </div>
  );
}

export function Steps({ children }: { children: ReactNode }) {
  return <ol className="not-prose mt-6 space-y-7">{children}</ol>;
}

export function Step({ n, title, children }: { n: number; title: string; children: ReactNode }) {
  return (
    <li className="relative pl-11">
      <span className="border-line bg-elevated text-fg absolute top-0 left-0 grid h-7 w-7 place-items-center rounded-full border font-mono text-xs">
        {n}
      </span>
      <h3 className="font-display text-fg text-base font-semibold tracking-tight">{title}</h3>
      <div className="text-muted mt-1 text-sm leading-relaxed">{children}</div>
    </li>
  );
}
