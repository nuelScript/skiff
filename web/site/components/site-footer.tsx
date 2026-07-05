import { Logo } from "@/components/logo";
import { DASHBOARD_URL } from "@/lib/site";

export function SiteFooter() {
  return (
    <footer className="border-line border-t">
      <div className="mx-auto flex max-w-6xl flex-col gap-6 px-6 py-10 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <Logo />
          <p className="text-muted mt-2 text-sm">
            Deploy anything, anywhere you own.
          </p>
        </div>
        <div className="text-subtle flex items-center gap-6 font-mono text-xs">
          <a href={DASHBOARD_URL} className="hover:text-fg transition-colors">
            CONSOLE
          </a>
          <a href="#features" className="hover:text-fg transition-colors">
            FEATURES
          </a>
          <span className="text-subtle/70">© SKIFF</span>
        </div>
      </div>
    </footer>
  );
}
