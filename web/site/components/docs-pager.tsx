"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { DOCS_FLAT } from "@/lib/docs-nav";

export function DocsPager() {
  const path = usePathname();
  const i = DOCS_FLAT.findIndex((x) => x.href === path);
  if (i === -1) return null;
  const prev = i > 0 ? DOCS_FLAT[i - 1] : null;
  const next = i < DOCS_FLAT.length - 1 ? DOCS_FLAT[i + 1] : null;
  if (!prev && !next) return null;

  return (
    <div className="border-line mt-16 grid gap-4 border-t pt-6 sm:grid-cols-2">
      {prev ? (
        <Link
          href={prev.href}
          className="border-line bg-surface hover:border-line-strong group rounded-xl border px-4 py-3 transition"
        >
          <span className="text-subtle flex items-center gap-1 font-mono text-[10px] tracking-wider uppercase">
            <ArrowLeft className="h-3 w-3" /> Previous
          </span>
          <span className="text-fg mt-1 block text-sm font-medium">{prev.label}</span>
        </Link>
      ) : (
        <div />
      )}
      {next ? (
        <Link
          href={next.href}
          className="border-line bg-surface hover:border-line-strong group rounded-xl border px-4 py-3 text-right transition"
        >
          <span className="text-subtle flex items-center justify-end gap-1 font-mono text-[10px] tracking-wider uppercase">
            Next <ArrowRight className="h-3 w-3" />
          </span>
          <span className="text-fg mt-1 block text-sm font-medium">{next.label}</span>
        </Link>
      ) : (
        <div />
      )}
    </div>
  );
}
