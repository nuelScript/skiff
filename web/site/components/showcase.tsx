"use client";

import { useRef } from "react";
import gsap from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import { useGSAP } from "@gsap/react";
import { DashboardPreview } from "@/components/dashboard-preview";

gsap.registerPlugin(ScrollTrigger, useGSAP);

export function Showcase() {
  const scope = useRef<HTMLElement>(null);

  useGSAP(
    () => {
      if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
      gsap.fromTo(
        ".dash-tilt",
        { rotateX: 16, y: 44, scale: 0.95 },
        {
          rotateX: 0,
          y: 0,
          scale: 1,
          ease: "none",
          scrollTrigger: {
            trigger: scope.current,
            start: "top 82%",
            end: "top 34%",
            scrub: true,
          },
        },
      );
    },
    { scope },
  );

  return (
    <section ref={scope} className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-2xl text-center">
        <p className="text-brand-2/80 font-mono text-[11px] tracking-[0.3em] uppercase">
          The console
        </p>
        <h2 className="font-display mt-4 text-3xl font-semibold tracking-tight text-balance sm:text-4xl">
          Your server gets a console.
        </h2>
        <p className="text-muted mt-4 text-balance">
          Deploy, watch live logs, roll back, open a shell, and manage every
          app, database, and domain from one place — the same box, a browser
          away.
        </p>
      </div>

      <div className="mt-16 [perspective:1600px]">
        <div className="dash-tilt origin-top">
          <DashboardPreview />
        </div>
      </div>
    </section>
  );
}
