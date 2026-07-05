import { BrowserFrame } from "@/components/browser-frame";
import { LogoMark } from "@/components/logo";

const apps = [
  { name: "api", state: "running", url: "api.useskiff.xyz" },
  { name: "web", state: "running", url: "web.useskiff.xyz" },
  { name: "worker", state: "building", url: "worker.useskiff.xyz" },
];

const dot: Record<string, string> = {
  running: "bg-fg",
  building: "bg-muted",
};

export function DashboardPreview() {
  return (
    <BrowserFrame url="dash.useskiff.xyz" className="glow-amber">
      <div className="bg-bg/40">
        <div className="border-line flex items-center justify-between border-b px-5 py-3.5">
          <span className="font-display flex items-center gap-2 text-sm font-bold">
            <LogoMark className="h-4 w-4" /> Skiff
          </span>
          <span className="btn-brand rounded-md px-3 py-1.5 text-xs font-semibold">
            Deploy from Git
          </span>
        </div>

        <div className="grid gap-3 p-5 sm:grid-cols-3">
          {apps.map((a) => (
            <div
              key={a.name}
              className="border-line bg-surface/60 rounded-xl border p-4"
            >
              <div className="flex items-center justify-between">
                <span className="text-fg text-sm font-medium">{a.name}</span>
                <span className="text-muted flex items-center gap-1.5 font-mono text-[10px] tracking-wide uppercase">
                  <span className={"h-1.5 w-1.5 rounded-full " + dot[a.state]} />
                  {a.state}
                </span>
              </div>
              <p className="text-subtle mt-3 truncate font-mono text-[11px]">
                {a.url}
              </p>
              <div className="text-subtle mt-3 flex gap-3 font-mono text-[11px]">
                <span className="hover:text-fg">logs</span>
                <span className="hover:text-fg">restart</span>
                <span className="hover:text-fg">stop</span>
              </div>
            </div>
          ))}
        </div>
      </div>
    </BrowserFrame>
  );
}
