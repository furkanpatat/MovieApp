"use client";

import { Layers, MessageCircle, Play, Radio } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { CommentSection, CommentSheet } from "@/components/movies/comment-section";
import { RatingSection } from "@/components/movies/rating-section";
import { CastRow } from "@/components/movies/cast-row";
import { MovieFacts } from "@/components/movies/omdb-info";
import { TrailerDialog } from "@/components/movies/trailer-dialog";
import {
  ErrorState,
  SaveButton,
  WatchedButton,
  SECONDARY_ACTION,
  TitleBackdrop,
  TitleDetailSkeleton,
  TitleInfoPanel,
  detailBodyClass,
  type DetailVariant,
} from "@/components/title/title-detail";
import { useInteractions, useTVDetails } from "@/hooks/queries";
import { plural, useT } from "@/i18n";
import { statusLabel } from "@/components/movies/omdb-info";
import { ApiError } from "@/lib/api-client";
import type { Movie } from "@/types/movie";

/**
 * A TV series: the movie layout with series facts (seasons, episodes,
 * status, network). Ratings, comments, My List and Watch Party are keyed by
 * movie id across the backend, so they are movie-only for now. Rendered by
 * /tv/[id] and by the @modal intercepting route.
 */
export function TVDetail({ showId, variant = "page" }: { showId: number; variant?: DetailVariant }) {
  const { t } = useT();
  const validId = Number.isInteger(showId) && showId > 0;
  const show = useTVDetails(validId ? showId : -1, validId);

  if (!validId) {
    return <ErrorState message={t("detail.invalidSeries")} />;
  }
  if (show.status === "pending") {
    return <TitleDetailSkeleton variant={variant} />;
  }
  if (show.status === "error") {
    const notFound = show.error instanceof ApiError && show.error.status === 404;
    return (
      <ErrorState
        message={t(notFound ? "detail.seriesNotFound" : "common.catalogUnreachable")}
        onRetry={notFound ? undefined : () => show.refetch()}
      />
    );
  }

  const s = show.data;
  return (
    <div className="flex flex-1 flex-col">
      <TitleBackdrop movie={s} variant={variant} />
      <div className={detailBodyClass(variant)}>
        <TitleInfoPanel movie={s} facts={<SeriesFacts show={s} />} actions={<SeriesActions show={s} />} />
        <div className="mt-12 space-y-14 pb-16">
          <CastRow castJson={s.cast_json} />
          <MovieFacts movie={s} />
          <SeriesRatingsAndComments show={s} />
        </div>
      </div>
    </div>
  );
}

/** Seasons, episodes and whether it's still running, in the meta row. */
function SeriesFacts({ show: s }: { show: Movie }) {
  const { t, locale } = useT();
  const running = s.status === "Returning Series" || s.status === "In Production";
  return (
    <>
      {s.number_of_seasons ? (
        <span className="flex items-center gap-1">
          <Layers className="size-3.5" />
          {t(plural(s.number_of_seasons, "detail.seasonsOne", "detail.seasonsOther"), { n: s.number_of_seasons })}
          {s.number_of_episodes ? <span className="text-zinc-400">· {t("detail.episodes", { n: s.number_of_episodes })}</span> : null}
        </span>
      ) : null}
      {s.status && (
        <span
          className={`flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold ring-1 ${
            running ? "bg-emerald-400/10 text-emerald-300 ring-emerald-400/30" : "bg-white/5 text-zinc-300 ring-white/15"
          }`}
        >
          <Radio className="size-3" />
          {statusLabel(locale, s.status)}
        </span>
      )}
    </>
  );
}

/** Trailer, My List and comments (keyed by media type, so series have
 *  their own, apart from the movie with the same id). */
function SeriesActions({ show: s }: { show: Movie }) {
  const { t } = useT();
  const trailerKey = s.trailer_key?.trim();
  return (
    <>
      {trailerKey && (
        <TrailerDialog
          videoKey={trailerKey}
          title={s.title}
          trigger={
            <Button size="lg" className="h-12 px-6 text-base font-semibold">
              <Play className="size-5 fill-current" />
              {t("detail.playTrailer")}
            </Button>
          }
        />
      )}
      <SaveButton movie={s} />
      <WatchedButton movie={s} />
      <CommentSheet
        subject={s}
        title={s.title}
        trigger={
          <Button size="lg" variant="secondary" className={SECONDARY_ACTION}>
            <MessageCircle className="size-5" />
            {t("detail.comments")}
          </Button>
        }
      />
    </>
  );
}

/** The community's stars, your own, and the comments: as on a movie page. */
function SeriesRatingsAndComments({ show: s }: { show: Movie }) {
  const { t } = useT();
  const interactions = useInteractions(s);
  return (
    <section aria-label={t("detail.ratingsAndComments")} className="max-w-4xl space-y-8">
      <h2 className="text-xl font-bold tracking-tight">{t("detail.ratingsAndComments")}</h2>
      {interactions.data ? (
        <>
          <RatingSection movie={s} interactions={interactions.data} />
          <CommentSection subject={s} interactions={interactions.data} />
        </>
      ) : (
        <div className="space-y-3">
          <Skeleton className="h-6 w-64 rounded" />
          <Skeleton className="h-20 w-full rounded" />
        </div>
      )}
    </section>
  );
}
