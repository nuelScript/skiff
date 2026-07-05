"use client";

import { useEffect, useState } from "react";
import { usePathname } from "next/navigation";

type Item = { id: string; text: string };

const slug = (s: string) =>
  s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");

export function DocsToc() {
  const path = usePathname();
  const [items, setItems] = useState<Item[]>([]);
  const [active, setActive] = useState("");

  useEffect(() => {
    const headings = Array.from(
      document.querySelectorAll<HTMLElement>("article h2"),
    );
    setItems(
      headings.map((h) => {
        if (!h.id) h.id = slug(h.textContent ?? "");
        return { id: h.id, text: h.textContent ?? "" };
      }),
    );
    const observer = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) setActive((e.target as HTMLElement).id);
        }
      },
      { rootMargin: "0px 0px -72% 0px" },
    );
    headings.forEach((h) => observer.observe(h));
    return () => observer.disconnect();
  }, [path]);

  if (items.length === 0) return null;
  return (
    <aside className="hidden w-44 shrink-0 xl:block">
      <div className="sticky top-24">
        <p className="text-subtle mb-3 font-mono text-[10px] tracking-[0.2em] uppercase">
          On this page
        </p>
        <ul className="border-line space-y-2 border-l">
          {items.map((it) => (
            <li key={it.id}>
              <a
                href={"#" + it.id}
                className={
                  "-ml-px block border-l pl-3 text-[13px] leading-snug transition " +
                  (active === it.id
                    ? "border-fg text-fg"
                    : "text-muted hover:text-fg border-transparent")
                }
              >
                {it.text}
              </a>
            </li>
          ))}
        </ul>
      </div>
    </aside>
  );
}
