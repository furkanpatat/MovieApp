import {
  Award,
  CalendarRange,
  Clapperboard,
  DollarSign,
  Globe,
  Languages,
  Layers,
  PenLine,
  Radio,
  Tv,
  type LucideIcon,
} from "lucide-react";

import { plural, translate, useT, type Locale, type MessageKey } from "@/i18n";
import { isTV } from "@/lib/media";

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
  const { t } = useT();
  return (
    <>
      {movie.rated && (
        <span
          className={cn("rounded border border-white/30 px-1.5 py-px text-xs font-semibold tracking-wide text-zinc-200", className)}
          title={t("rating.ageRating")}
        >
          {movie.rated}
        </span>
      )}
      {movie.rotten_tomatoes && (
        <span
          className={cn("inline-flex items-center gap-1 text-sm font-semibold text-white", className)}
          title={t("rating.tomatometer")}
          aria-label={`Rotten Tomatoes ${movie.rotten_tomatoes}`}
        >
          <span aria-hidden className={cn("inline-block size-3.5 rounded-full", fresh ? "bg-[#fa320a]" : "bg-[#0ac855]")} />
          {movie.rotten_tomatoes}
        </span>
      )}
      {movie.metascore ? (
        <span
          className={cn("inline-flex items-center gap-1.5 text-xs text-zinc-300", className)}
          title={t("rating.metascore")}
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

type Fact = { label: MessageKey; icon: LucideIcon; value: (m: Movie, locale: Locale) => string | undefined };

const omdb = (key: "director" | "writer" | "awards" | "box_office" | "country" | "language") => (m: Movie) => m[key];

const MOVIE_FACTS: Fact[] = [
  { label: "facts.director", icon: Clapperboard, value: omdb("director") },
  { label: "facts.writers", icon: PenLine, value: omdb("writer") },
  { label: "facts.awards", icon: Award, value: omdb("awards") },
  { label: "facts.boxOffice", icon: DollarSign, value: omdb("box_office") },
  { label: "facts.country", icon: Globe, value: omdb("country") },
  { label: "facts.language", icon: Languages, value: omdb("language") },
];

/** A TMDB series status ("Returning Series") in the UI language. */
export function statusLabel(locale: Locale, status: string): string {
  const key = `status.${status}` as MessageKey;
  const text = translate(locale, key);
  return text === key ? status : text;
}

/** "2011 - 2019", or "2022 - present" while the series is still running. */
function airedLabel(m: Movie, locale: Locale) {
  const first = m.release_date?.slice(0, 4);
  if (!first) return undefined;
  const last = m.last_air_date?.slice(0, 4);
  if (m.status === "Returning Series" || m.status === "In Production") return `${first} – ${translate(locale, "facts.present")}`;
  return last && last !== first ? `${first} – ${last}` : first;
}

function episodesLabel(m: Movie, locale: Locale) {
  const parts = [];
  if (m.number_of_seasons) parts.push(translate(locale, plural(m.number_of_seasons, "detail.seasonsOne", "detail.seasonsOther"), { n: m.number_of_seasons }));
  if (m.number_of_episodes) parts.push(translate(locale, "detail.episodes", { n: m.number_of_episodes }));
  return parts.join(" · ") || undefined;
}

const TV_FACTS: Fact[] = [
  { label: "facts.createdBy", icon: Clapperboard, value: (m) => m.creators?.join(", ") },
  { label: "facts.network", icon: Tv, value: (m) => m.networks?.join(", ") },
  { label: "facts.seasons", icon: Layers, value: episodesLabel },
  { label: "facts.aired", icon: CalendarRange, value: airedLabel },
  { label: "facts.status", icon: Radio, value: (m, locale) => (m.status ? statusLabel(locale, m.status) : undefined) },
  { label: "facts.writers", icon: PenLine, value: omdb("writer") },
  { label: "facts.awards", icon: Award, value: omdb("awards") },
  { label: "facts.country", icon: Globe, value: omdb("country") },
  { label: "facts.language", icon: Languages, value: omdb("language") },
];

/** Credits and facts (OMDb's, plus TMDB's series data for a series);
 *  hidden when there are none. OMDb's own values stay as OMDb writes them. */
export function MovieFacts({ movie }: { movie: Movie }) {
  const { t, locale } = useT();
  const facts = (isTV(movie) ? TV_FACTS : MOVIE_FACTS)
    .map((f) => ({ ...f, text: f.value(movie, locale) }))
    .filter((f) => f.text);
  if (facts.length === 0) return null;
  return (
    <section aria-labelledby="movie-facts-heading">
      <h2 id="movie-facts-heading" className="text-xl font-bold tracking-tight">
        {t(isTV(movie) ? "facts.aboutSeries" : "facts.aboutFilm")}
      </h2>
      <dl className="mt-4 grid gap-x-8 gap-y-5 sm:grid-cols-2 lg:grid-cols-3">
        {facts.map(({ label, icon: Icon, text }) => (
          <div key={label} className="flex gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-white/5 text-primary ring-1 ring-white/10">
              <Icon className="size-4" />
            </span>
            <div className="min-w-0">
              <dt className="text-xs text-muted-foreground">{t(label)}</dt>
              <dd className="text-sm font-medium text-zinc-100">{text}</dd>
            </div>
          </div>
        ))}
      </dl>
    </section>
  );
}
