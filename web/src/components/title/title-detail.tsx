"use client";

import type { ReactNode } from "react";
import Image from "next/image";
import { Bookmark, BookmarkCheck, CircleCheck, Clapperboard, Clock, Eye, TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { RatingBadge } from "@/components/movies/rating-badge";
import { CriticScores } from "@/components/movies/omdb-info";
import { TVBadge } from "@/components/movies/movie-card";
import { useSavedToggle, useWatchedToggle } from "@/hooks/use-library";
import { genreName, useT } from "@/i18n";
import { isTV } from "@/lib/media";
import { backdropUrl, posterUrl } from "@/lib/tmdb-image";
import { cn } from "@/lib/utils";
import type { Movie } from "@/types/movie";

/**
 * The layout shared by movie and TV series pages: a full-bleed backdrop,
 * then a poster and glass info panel riding up over its bottom edge.
 */

/** Where the details render: their own page, or the modal over a list
 *  (@modal intercepting routes), which gets a shorter hero. */
export type DetailVariant = "page" | "modal";

// The hero's height and how far the info panel rides up over it. Shared with
// the skeleton so loading -> loaded doesn't shift the layout.
const GEOMETRY: Record<DetailVariant, { hero: string; overlap: string }> = {
  page: { hero: "h-[60vh] min-h-[420px] md:h-[80vh]", overlap: "-mt-[26vh] md:-mt-[40vh]" },
  modal: { hero: "h-[42vh] min-h-[300px] md:h-[58vh]", overlap: "-mt-[18vh] md:-mt-[28vh]" },
};

/** The content column under the hero, riding up over it. */
export function detailBodyClass(variant: DetailVariant) {
  return `relative z-10 mx-auto w-full max-w-screen-2xl px-4 sm:px-8 ${GEOMETRY[variant].overlap}`;
}

export function runtimeLabel(minutes?: number): string | null {
  if (!minutes) return null;
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

/** Full-bleed backdrop that dissolves into the page below it. */
export function TitleBackdrop({ movie, variant = "page" }: { movie: Movie; variant?: DetailVariant }) {
  const backdrop = backdropUrl(movie.backdrop_path, "original");
  return (
    <section className={cn("relative w-full overflow-hidden", GEOMETRY[variant].hero)} aria-hidden>
      {backdrop ? (
        <Image src={backdrop} alt="" fill priority sizes="100vw" className="object-cover object-top" />
      ) : (
        <div className="auth-aurora absolute inset-0" />
      )}
      <div className="scrim-top absolute inset-x-0 top-0 h-32" />
      <div className="absolute inset-0 bg-gradient-to-r from-background/80 via-background/20 to-transparent" />
      <div className="absolute inset-0 bg-gradient-to-t from-background via-background/60 to-transparent" />
    </section>
  );
}

/**
 * Poster + glass panel floating over the bottom edge of the backdrop.
 * `facts` extend the meta row (after rating, year and runtime); `actions`
 * are the buttons under the overview.
 */
export function TitleInfoPanel({ movie: m, facts, actions }: { movie: Movie; facts?: ReactNode; actions?: ReactNode }) {
  const poster = posterUrl(m.poster_path, "w500");
  const year = m.release_date?.slice(0, 4);
  const runtime = runtimeLabel(m.runtime);
  const { t, locale } = useT();

  return (
    <div className="flex flex-col gap-6 md:flex-row md:items-end md:gap-8">
      <div className="relative aspect-2/3 w-32 shrink-0 overflow-hidden rounded-xl bg-zinc-900 shadow-2xl shadow-black/60 ring-1 ring-white/10 sm:w-44 md:w-60 lg:w-64">
        {poster ? (
          <Image src={poster} alt={m.title} fill priority sizes="(min-width: 768px) 256px, 176px" className="object-cover" />
        ) : (
          <div className="flex size-full items-center justify-center">
            <Clapperboard className="size-10 text-muted-foreground" strokeWidth={1.5} />
          </div>
        )}
      </div>

      <div className="min-w-0 flex-1 rounded-2xl border border-white/10 bg-zinc-950/55 p-6 shadow-2xl shadow-black/40 backdrop-blur-xl md:p-8">
        {isTV(m) && <TVBadge className="mb-3" />}
        <h1 className="text-3xl font-bold tracking-tight text-balance sm:text-4xl lg:text-5xl">{m.title}</h1>
        {m.tagline && <p className="mt-2 text-base text-zinc-300 italic sm:text-lg">{m.tagline}</p>}

        <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-zinc-300">
          <RatingBadge movie={m} size="lg" showVotes />
          <CriticScores movie={m} />
          {year && <span>{year}</span>}
          {runtime && (
            <span className="flex items-center gap-1">
              <Clock className="size-3.5" />
              {runtime}
              {isTV(m) && <span className="text-zinc-400">{t("detail.perEpisode")}</span>}
            </span>
          )}
          {facts}
        </div>

        {m.genres && m.genres.length > 0 && (
          <ul className="mt-4 flex flex-wrap gap-2" aria-label={t("detail.genres")}>
            {m.genres.map((g) => (
              <li key={g.id} className="rounded-full border border-white/15 bg-white/5 px-3 py-1 text-xs font-medium text-zinc-200">
                {genreName(locale, g)}
              </li>
            ))}
          </ul>
        )}

        {m.overview && <p className="mt-5 max-w-3xl leading-relaxed text-zinc-200">{m.overview}</p>}

        {actions && <div className="mt-6 flex flex-wrap gap-3">{actions}</div>}
      </div>
    </div>
  );
}

export const SECONDARY_ACTION = "h-12 bg-white/10 px-6 text-base font-semibold text-white hover:bg-white/20";

/** Mark a movie or series watched (or not); opens sign-in when signed out.
 *  Watched titles are on the user's public profile. */
export function WatchedButton({ movie }: { movie: Movie }) {
  const { t } = useT();
  const { watched, toggle } = useWatchedToggle(movie);
  return (
    <Button
      size="lg"
      variant="secondary"
      className={cn(SECONDARY_ACTION, watched && "bg-emerald-500/15 text-emerald-300 ring-1 ring-emerald-400/30 hover:bg-emerald-500/25")}
      onClick={toggle}
      aria-pressed={watched}
    >
      {watched ? <CircleCheck className="size-5" /> : <Eye className="size-5" />}
      {t(watched ? "detail.watched" : "detail.markWatched")}
    </Button>
  );
}

/** Add to / remove from My List (a movie or a series); opens sign-in when
 *  signed out. */
export function SaveButton({ movie }: { movie: Movie }) {
  const { t } = useT();
  const { saved, toggle } = useSavedToggle(movie);
  return (
    <Button size="lg" variant="secondary" className={SECONDARY_ACTION} onClick={toggle} aria-pressed={saved}>
      {saved ? <BookmarkCheck className="size-5 text-primary" /> : <Bookmark className="size-5" />}
      {t(saved ? "detail.inMyList" : "detail.myList")}
    </Button>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-4 px-6 py-24 text-center">
      <TriangleAlert className="size-10 text-destructive" strokeWidth={1.5} />
      <p className="text-lg font-semibold">{message}</p>
      {onRetry && (
        <Button variant="secondary" onClick={onRetry}>
          Try again
        </Button>
      )}
    </div>
  );
}

/** Loading state in the page's exact geometry. */
export function TitleDetailSkeleton({ variant = "page" }: { variant?: DetailVariant }) {
  return (
    <div className="flex flex-1 flex-col">
      <Skeleton className={cn("w-full rounded-none", GEOMETRY[variant].hero)} />
      <div className={detailBodyClass(variant)}>
        <div className="flex flex-col gap-6 md:flex-row md:items-end md:gap-8">
          <Skeleton className="aspect-2/3 w-32 shrink-0 rounded-xl sm:w-44 md:w-60 lg:w-64" />
          <div className="flex-1 space-y-4 rounded-2xl border border-white/10 bg-zinc-950/55 p-6 md:p-8">
            <Skeleton className="h-10 w-2/3 rounded" />
            <Skeleton className="h-5 w-1/3 rounded" />
            <Skeleton className="h-20 w-full rounded" />
            <Skeleton className="h-12 w-48 rounded-lg" />
          </div>
        </div>
      </div>
    </div>
  );
}
