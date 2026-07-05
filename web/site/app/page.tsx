import { SiteBackground } from "@/components/site-background";
import { SiteNav } from "@/components/site-nav";
import { Hero } from "@/components/hero";
import { StackMarquee } from "@/components/stack-marquee";
import { StatsStrip } from "@/components/stats-strip";
import { FeatureRows } from "@/components/feature-rows";
import { Showcase } from "@/components/showcase";
import { HowItWorks } from "@/components/how-it-works";
import { FinalCta } from "@/components/final-cta";
import { SiteFooter } from "@/components/site-footer";
import { Reveal } from "@/components/reveal";

export default function Home() {
  return (
    <>
      <SiteBackground />
      <SiteNav />
      <main>
        <Hero />
        <Reveal>
          <StackMarquee />
        </Reveal>
        <Reveal>
          <StatsStrip />
        </Reveal>
        <Reveal>
          <FeatureRows />
        </Reveal>
        <Showcase />
        <Reveal>
          <HowItWorks />
        </Reveal>
        <Reveal>
          <FinalCta />
        </Reveal>
      </main>
      <SiteFooter />
    </>
  );
}
