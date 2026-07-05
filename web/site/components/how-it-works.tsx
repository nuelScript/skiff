import { SectionHeading } from "@/components/section-heading";

const steps = [
  {
    n: "01",
    title: "Point Skiff at your server",
    body: "One command connects Skiff to any Linux box over SSH.",
  },
  {
    n: "02",
    title: "Push your code",
    body: "Deploy from a git push or straight from the console.",
  },
  {
    n: "03",
    title: "Get an HTTPS URL",
    body: "Skiff builds, runs, and serves your app on your domain.",
  },
];

export function HowItWorks() {
  return (
    <section id="how" className="mx-auto max-w-6xl px-6 py-24">
      <SectionHeading
        eyebrow="The passage"
        title="From zero to deployed in three moves."
      />
      <div className="mt-14 grid gap-10 sm:grid-cols-3">
        {steps.map((s) => (
          <div key={s.n}>
            <div className="flex items-center gap-3">
              <span className="font-display text-brand-2 text-2xl font-bold">
                {s.n}
              </span>
              <span className="from-line-strong h-px flex-1 bg-gradient-to-r to-transparent" />
            </div>
            <h3 className="font-display mt-5 text-lg font-semibold">{s.title}</h3>
            <p className="text-muted mt-2 text-sm leading-relaxed">{s.body}</p>
          </div>
        ))}
      </div>
    </section>
  );
}
