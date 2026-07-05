import type { ReactNode } from "react";
import { SiteBackground } from "@/components/site-background";
import { SiteNav } from "@/components/site-nav";
import { SiteFooter } from "@/components/site-footer";
import { DocsSidebar } from "@/components/docs-sidebar";

export default function DocsLayout({ children }: { children: ReactNode }) {
  return (
    <>
      <SiteBackground />
      <SiteNav />
      <div className="mx-auto flex max-w-6xl gap-12 px-6 py-12">
        <DocsSidebar />
        <main className="min-w-0 max-w-3xl flex-1">{children}</main>
      </div>
      <SiteFooter />
    </>
  );
}
