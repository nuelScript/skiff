export function FinalCta() {
  return (
    <section className="mx-auto max-w-6xl px-6 pb-24">
      <div className="ring-glow border-line-strong/60 bg-surface relative overflow-hidden rounded-3xl border px-8 py-16 text-center">
        <div className="hero-glow pointer-events-none absolute inset-0 opacity-70" />
        <div className="grid-overlay pointer-events-none absolute inset-0" />
        <div className="relative">
          <p className="text-signal font-mono text-[11px] tracking-[0.3em] uppercase">
            Cast off
          </p>
          <h2 className="font-display mx-auto mt-4 max-w-xl text-4xl font-bold tracking-tight text-balance sm:text-5xl">
            Ship on your own terms.
          </h2>
          <p className="text-muted mx-auto mt-4 max-w-md">
            No cloud bill. No lock-in. Just your code, your server, and one
            command.
          </p>
          <div className="mt-8 flex items-center justify-center gap-3">
            <a
              href="/docs"
              className="btn-brand rounded-lg px-5 py-2.5 text-sm font-semibold"
            >
              Get started
            </a>
            <a
              href="#how"
              className="border-line-strong/70 bg-bg/40 text-fg hover:bg-elevated rounded-lg border px-5 py-2.5 text-sm font-medium transition-colors"
            >
              See how it works
            </a>
          </div>
        </div>
      </div>
    </section>
  );
}
