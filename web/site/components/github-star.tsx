import { FaGithub } from "react-icons/fa";
import { Star } from "lucide-react";
import { GITHUB_REPO, GITHUB_URL } from "@/lib/site";

// Fetch the repo's star count, cached for an hour so we stay well under GitHub's
// unauthenticated rate limit. Any failure just drops the count — the button
// still renders.
async function getStars(): Promise<number | null> {
  try {
    const res = await fetch(`https://api.github.com/repos/${GITHUB_REPO}`, {
      next: { revalidate: 3600 },
      headers: { Accept: "application/vnd.github+json" },
    });
    if (!res.ok) return null;
    const data = (await res.json()) as { stargazers_count?: number };
    return typeof data.stargazers_count === "number" ? data.stargazers_count : null;
  } catch {
    return null;
  }
}

function formatStars(n: number): string {
  if (n < 1000) return String(n);
  return (n / 1000).toFixed(1).replace(/\.0$/, "") + "k";
}

export async function GitHubStars() {
  const stars = await getStars();
  return (
    <a
      href={GITHUB_URL}
      target="_blank"
      rel="noreferrer"
      aria-label="Star Skiff on GitHub"
      className="border-line/70 text-muted hover:text-fg hover:border-fg/25 flex items-center gap-2 rounded-lg border px-3 py-2 text-sm transition-colors"
    >
      <FaGithub className="h-4 w-4" />
      <span className="hidden font-medium sm:inline">Star</span>
      {stars != null && (
        <span className="border-line/60 flex items-center gap-1 border-l pl-2 font-mono text-xs tabular-nums">
          <Star className="h-3 w-3 fill-current" />
          {formatStars(stars)}
        </span>
      )}
    </a>
  );
}
