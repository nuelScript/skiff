const stats = [
  { n: "12+", l: "stacks, zero config" },
  { n: "<60s", l: "to automatic HTTPS" },
  { n: "0", l: "downtime deploys" },
  { n: "100%", l: "your hardware" },
];

export function StatsStrip() {
  return (
    <section className="mx-auto max-w-6xl px-6">
      <div className="border-line bg-line grid grid-cols-2 gap-px overflow-hidden rounded-2xl border md:grid-cols-4">
        {stats.map((s) => (
          <div key={s.l} className="bg-bg px-6 py-8 text-center">
            <p className="font-display text-fg text-4xl font-semibold tracking-tight">
              {s.n}
            </p>
            <p className="text-muted mt-2 font-mono text-[11px] tracking-wider uppercase">
              {s.l}
            </p>
          </div>
        ))}
      </div>
    </section>
  );
}
