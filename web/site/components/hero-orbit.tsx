"use client";

import { useRef } from "react";
import gsap from "gsap";
import { useGSAP } from "@gsap/react";
import type { IconType } from "react-icons";
import {
  SiNodedotjs,
  SiPython,
  SiGo,
  SiRust,
  SiRuby,
  SiElixir,
  SiPhp,
  SiDotnet,
  SiNextdotjs,
  SiVite,
  SiHtml5,
} from "react-icons/si";
import { FaJava } from "react-icons/fa";
import { LogoTile } from "@/components/logo";

gsap.registerPlugin(useGSAP);

type Item = { Icon: IconType; label: string };

const inner: Item[] = [
  { Icon: SiNodedotjs, label: "Node.js" },
  { Icon: SiGo, label: "Go" },
  { Icon: SiRust, label: "Rust" },
  { Icon: SiPython, label: "Python" },
];
const mid: Item[] = [
  { Icon: SiNextdotjs, label: "Next.js" },
  { Icon: SiVite, label: "Vite" },
  { Icon: SiRuby, label: "Ruby" },
  { Icon: SiElixir, label: "Elixir" },
];
const outer: Item[] = [
  { Icon: SiPhp, label: "PHP" },
  { Icon: FaJava, label: "Java" },
  { Icon: SiDotnet, label: ".NET" },
  { Icon: SiHtml5, label: "Static" },
];

const ringPct = [18, 30, 42];

function Tile({ Icon, label }: Item) {
  return (
    <div
      title={label}
      className="orbit-tile glass border-line text-fg/80 flex h-11 w-11 items-center justify-center rounded-xl border shadow-lg shadow-black/40"
    >
      <Icon className="h-5 w-5" />
    </div>
  );
}

function Orbit({
  radiusPct,
  durationS,
  reverse = false,
  items,
}: {
  radiusPct: number;
  durationS: number;
  reverse?: boolean;
  items: Item[];
}) {
  const n = items.length;
  return (
    <div
      className={"absolute inset-0 " + (reverse ? "orbit-spin-rev" : "orbit-spin")}
      style={{ animationDuration: `${durationS}s` }}
    >
      {items.map((it, i) => {
        const a = (2 * Math.PI * i) / n - Math.PI / 2;
        const left = 50 + radiusPct * Math.cos(a);
        const top = 50 + radiusPct * Math.sin(a);
        return (
          <div
            key={it.label}
            className="absolute"
            style={{ left: `${left}%`, top: `${top}%`, transform: "translate(-50%,-50%)" }}
          >
            <div
              className={reverse ? "orbit-spin" : "orbit-spin-rev"}
              style={{ animationDuration: `${durationS}s` }}
            >
              <Tile {...it} />
            </div>
          </div>
        );
      })}
    </div>
  );
}

export function HeroOrbit() {
  const scope = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
      gsap.from(".orbit-ring", {
        scale: 0.6,
        opacity: 0,
        transformOrigin: "center",
        duration: 1,
        stagger: 0.12,
        ease: "power2.out",
      });
      gsap.from(".orbit-core", {
        scale: 0,
        opacity: 0,
        duration: 0.8,
        ease: "back.out(1.8)",
        delay: 0.15,
      });
      gsap.from(".orbit-tile", {
        scale: 0,
        opacity: 0,
        duration: 0.6,
        stagger: 0.05,
        delay: 0.35,
        ease: "back.out(2)",
      });
    },
    { scope },
  );

  return (
    <div ref={scope} className="relative aspect-square w-full max-w-[480px]">
      <div
        className="absolute inset-[14%] rounded-full blur-md"
        style={{
          background:
            "radial-gradient(circle, rgba(var(--glow),0.1), transparent 60%)",
        }}
      />

      <div className="absolute inset-0 grid place-items-center">
        {ringPct.map((p, i) => (
          <div
            key={p}
            className="orbit-ring border-line/70 rounded-full border"
            style={{
              width: `${2 * p}%`,
              height: `${2 * p}%`,
              borderStyle: i === ringPct.length - 1 ? "dashed" : "solid",
            }}
          />
        ))}
      </div>

      <Orbit radiusPct={ringPct[0]} durationS={34} items={inner} />
      <Orbit radiusPct={ringPct[1]} durationS={48} reverse items={mid} />
      <Orbit radiusPct={ringPct[2]} durationS={64} items={outer} />

      <div className="absolute inset-0 grid place-items-center">
        <div className="orbit-core">
          <div
            className="animate-core rounded-2xl"
            style={{
              boxShadow:
                "0 0 55px -6px rgba(var(--glow),0.3), 0 10px 34px -8px rgba(0,0,0,0.55)",
            }}
          >
            <LogoTile className="h-16 w-16" />
          </div>
        </div>
      </div>
    </div>
  );
}
