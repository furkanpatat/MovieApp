import { Award, Clapperboard, DollarSign, Globe, Languages, PenLine } from "lucide-react";

import { cn } from "@/lib/utils";
import type { Movie } from "@/types/movie";

/** Metacritic's own bands: 61+ favourable, 40-60 mixed, below unfavourable. */
function metascoreColor(n: number) {
  if (n >= 61) return "bg-[#66cc33] text-black";
  if (n >= 40) return "bg-[#ffcc33] text-black";
  return "bg-[#ff0000] text-white";
}

/** Age rating plus critics' scores, next to the audience rating badge. */
export function CriticScores({ movie, className }: { movie: Movie; className?: string }) {
  const tomatometer = movie.rotten_tomatoes ? parseInt(movie.rotten_tomatoes, 10) : NaN;
  const fresh = tomatometer >= 60;
  return (
    <>
      {movie.rated && (
        <span
          className={cn("rounded border border-white/30 px-1.5 py-px text-xs font-semibold tracking-wide text-zinc-200", className)}
          title="Age rating"
        >
          {movie.rated}
        </span>
      )}
      {movie.rotten_tomatoes && (
        <span
          className={cn("inline-flex items-center gap-1 text-sm font-semibold text-white", className)}
          title="Rotten Tomatoes Tomatometer"
          aria-label={`Rotten Tomatoes ${movie.rotten_tomatoes}`}
        >
          <span aria-hidden className={cn("inline-block size-3.5 rounded-full", fresh ? "bg-[#fa320a]" : "bg-[#0ac855]")} />
          {movie.rotten_tomatoes}
        </span>
      )}
      {movie.metascore ? (
        <span
          className={cn("inline-flex items-center gap-1.5 text-xs text-zinc-300", className)}
          title="Metacritic Metascore"
          aria-label={`Metascore ${movie.metascore}`}
        >
          <span aria-hidden className={cn("flex size-6 items-center justify-center rounded text-xs font-bold", metascoreColor(movie.metascore))}>
            {movie.metascore}
          </span>
          Metascore
        </span>
      ) : null}
    </>
  );
}

const FACTS = [
  { key: "director", label: "Director", icon: Clapperboard },
  { key: "writer", label: "Writers", icon: PenLine },
  { key: "awards", label: "Awards", icon: Award },
  { key: "box_office", label: "Box office (US)", icon: DollarSign },
  { key: "country", label: "Country", icon: Globe },
  { key: "language", label: "Language", icon: Languages },
] as const;

/** Credits and facts from OMDb; hidden when there are none. */
export function MovieFacts({ movie }: { movie: Movie }) {
  const facts = FACTS.filter((f) => movie[f.key]);
  if (facts.length === 0) return null;
  return (
    <section aria-labelledby="movie-facts-heading">
      <h2 id="movie-facts-heading" className="text-xl font-bold tracking-tight">
        About the film
      </h2>
      <dl className="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 lg:grid-cols-3">
        {facts.map(({ key, label, icon: Icon }) => (
          <div key={key} className="flex gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-white/5 text-primary ring-1 ring-white/10">
              <Icon className="size-4" />
            </span>
            <div className="min-w-0">
              <dt className="text-xs text-muted-foreground">{label}</dt>
              <dd className="text-sm font-medium text-zinc-100">{movie[key]}</dd>
            </div>
          </div>
        ))}
      </dl>
    </section>
  );
}
