"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { DOCS_NAV } from "@/lib/docs-nav";

export function DocsSidebar() {
  const path = usePathname();
  return (
    <aside className="hidden w-52 shrink-0 lg:block">
      <nav className="sticky top-24 space-y-6">
        {DOCS_NAV.map((section) => (
          <div key={section.title}>
            <p className="text-subtle mb-2 font-mono text-[10px] tracking-[0.2em] uppercase">
              {section.title}
            </p>
            <ul className="space-y-0.5">
              {section.items.map((item) => {
                const active = path === item.href;
                return (
                  <li key={item.href}>
                    <Link
                      href={item.href}
                      className={
                        "block rounded-md px-2 py-1 text-sm transition " +
                        (active ? "bg-elevated text-fg" : "text-muted hover:text-fg")
                      }
                    >
                      {item.label}
                    </Link>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>
    </aside>
  );
}
