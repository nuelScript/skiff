"use client";

import { useRef } from "react";
import gsap from "gsap";
import { useGSAP } from "@gsap/react";
import { ArrowRight, Copy } from "lucide-react";
import { HeroOrbit } from "@/components/hero-orbit";
import { DASHBOARD_URL } from "@/lib/site";

gsap.registerPlugin(useGSAP);

export function Hero() {
  const scope = useRef<HTMLElement>(null);

  useGSAP(
    () => {
      if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
      gsap.from(".hero-item", {
        opacity: 0,
        y: 22,
        duration: 0.9,
        stagger: 0.09,
        ease: "power3.out",
      });
    },
    { scope },
  );

  return (
    <section
      ref={scope}
      className="relative mx-auto grid max-w-6xl items-center gap-14 px-6 pt-20 pb-24 lg:grid-cols-[1.05fr_1fr] lg:gap-8 lg:pt-28"
    >
      <div>
        <div className="hero-item border-line bg-surface/60 text-muted inline-flex items-center gap-2 rounded-full border px-3 py-1 font-mono text-xs">
          <span className="animate-pulse-dot bg-signal h-1.5 w-1.5 rounded-full" />
          Self-hosted deploys
        </div>

        <h1 className="hero-item font-display mt-6 text-5xl leading-[0.98] font-semibold tracking-tight sm:text-6xl md:text-[4.75rem]">
          Ship it to a <span className="text-gradient">server you own.</span>
        </h1>

        <p className="hero-item text-muted mt-6 max-w-md text-lg leading-relaxed">
          Push-to-deploy with automatic HTTPS, managed databases, and preview
          environments — on infrastructure you control, not rented.
        </p>

        <div className="hero-item mt-8 flex flex-wrap items-center gap-3">
          <a
            href={DASHBOARD_URL}
            className="btn-brand rounded-lg px-5 py-2.5 text-sm font-semibold"
          >
            Launch the console
          </a>
          <a
            href="#features"
            className="group border-line-strong/70 bg-bg/40 text-fg hover:bg-elevated inline-flex items-center gap-1.5 rounded-lg border px-5 py-2.5 text-sm font-medium transition-colors"
          >
            See how it works
            <ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
          </a>
        </div>

        <div className="hero-item border-line bg-surface/70 text-muted mt-8 flex max-w-sm items-center justify-between gap-3 rounded-lg border px-4 py-2.5 font-mono text-[13px]">
          <span className="truncate">
            <span className="text-brand">$</span> curl -fsSL useskiff.xyz/boot |
            sh
          </span>
          <button
            type="button"
            aria-label="Copy install command"
            onClick={() =>
              navigator.clipboard?.writeText("curl -fsSL useskiff.xyz/boot | sh")
            }
            className="text-subtle hover:text-fg shrink-0 transition-colors"
          >
            <Copy className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      <div className="hero-item relative flex items-center justify-center">
        <HeroOrbit />
      </div>
    </section>
  );
}
