import { Logo } from "@/components/logo";
import { DASHBOARD_URL } from "@/lib/site";

const links = [
  { href: "/#features", label: "FEATURES" },
  { href: "/#how", label: "HOW IT WORKS" },
  { href: "/#stacks", label: "STACKS" },
  { href: "/docs", label: "DOCS" },
];

export function SiteNav() {
  return (
    <header className="border-line/60 bg-bg/70 sticky top-0 z-50 border-b backdrop-blur-xl">
      <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
        <Logo />
        <nav className="text-muted hidden items-center gap-8 font-mono text-xs tracking-wide md:flex">
          {links.map((l) => (
            <a key={l.href} href={l.href} className="hover:text-fg transition-colors">
              {l.label}
            </a>
          ))}
        </nav>
        <a
          href={DASHBOARD_URL}
          className="btn-brand rounded-lg px-4 py-2 text-sm font-semibold"
        >
          Launch console
        </a>
      </div>
    </header>
  );
}
