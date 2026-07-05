"use client";

import { useState } from "react";
import { Check, Copy } from "lucide-react";

export function CodeBlock({ children, label }: { children: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="not-prose border-line bg-surface group relative my-5 overflow-hidden rounded-xl border">
      {label ? (
        <div className="border-line/60 text-subtle border-b px-4 py-1.5 font-mono text-[10px] tracking-wider uppercase">
          {label}
        </div>
      ) : null}
      <button
        type="button"
        aria-label="Copy"
        onClick={() => {
          navigator.clipboard?.writeText(children);
          setCopied(true);
          setTimeout(() => setCopied(false), 1200);
        }}
        className="border-line/60 bg-elevated text-subtle hover:text-fg absolute top-2.5 right-2.5 cursor-pointer rounded-md border p-1.5 opacity-0 transition group-hover:opacity-100"
      >
        {copied ? (
          <Check className="text-fg h-3.5 w-3.5" />
        ) : (
          <Copy className="h-3.5 w-3.5" />
        )}
      </button>
      <pre className="text-fg/90 overflow-x-auto px-4 py-3.5 font-mono text-[13px] leading-relaxed">
        <code>{children}</code>
      </pre>
    </div>
  );
}
