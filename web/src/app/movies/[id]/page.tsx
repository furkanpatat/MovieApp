"use client";

import Image from "next/image";
import dynamic from "next/dynamic";
import { useParams } from "next/navigation";
import { Bookmark, BookmarkCheck, Clapperboard, Clock, MessageCircle, Play, TriangleAlert, Users } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { CastRow } from "@/components/movies/cast-row";
import { CommentSheet } from "@/components/movies/comment-section";
import { RatingSection } from "@/components/movies/rating-section";
import { RatingBadge } from "@/components/movies/rating-badge";
import { CriticScores, MovieFacts } from "@/components/movies/omdb-info";
import { TrailerDialog } from "@/components/movies/trailer-dialog";
import { VideoPlayerPlaceholder } from "@/components/movies/video-player-placeholder";
import { useInteractions, useMovieDetails } from "@/hooks/queries";
import { useSavedToggle } from "@/hooks/use-library";
import { useWatchParty } from "@/hooks/use-watch-party";
import { ApiError } from "@/lib/api-client";
import { backdropUrl, posterUrl } from "@/lib/tmdb-image";
import { useAuthStore } from "@/store/auth-store";
import type { Movie } from "@/types/movie";

const CommentSection = dynamic(() => import("@/components/movies/comment-section").then((mod) => mod.CommentSection), { ssr: false });
const WatchPartyPanel = dynamic(() => import("@/components/watch-party/watch-party-panel").then((mod) => mod.WatchPartyPanel), { ssr: false });

function runtimeLabel(minutes?: number): string | null {
  if (!minutes) return null;
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

// The hero's height and how far the info panel rides up over it. Shared with
// the skeleton so loading -> loaded doesn't shift the layout.
const HERO = "h-[60vh] min-h-[420px] md:h-[80vh]";
const OVERLAP = "-mt-[26vh] md:-mt-[40vh]";

export default function MovieDetailPage() {
  const params = useParams<{ id: string }>();
  const movieId = Number(params.id);
  const validId = Number.isInteger(movieId) && movieId > 0;

  const isAuthed = useAuthStore((s) => s.hasHydrated && s.username !== null);
  const currentUserId = useAuthStore((s) => s.userId);

  const movie = useMovieDetails(validId ? movieId : -1);
  const interactions = useInteractions(validId ? movieId : -1);
  // One socket for the whole page: the synced player's play/pause and the
  // chat panel both read/drive this same connection.
  const wp = useWatchParty(movieId, isAuthed);

  if (!validId) {
    return <ErrorState message="That doesn't look like a valid movie link." />;
  }

  if (movie.status === "pending") {
    return <MovieDetailSkeleton />;
  }

  if (movie.status === "error") {
    const notFound = movie.error instanceof ApiError && movie.error.status === 404;
    return (
      <ErrorState
        message={notFound ? "We couldn't find that movie." : "The catalog is unreachable right now."}
        onRetry={notFound ? undefined : () => movie.refetch()}
      />
    );
  }

  const m = movie.data;

  return (
    <div className="flex flex-1 flex-col">
      <Backdrop movie={m} />

      <div className={`relative z-10 mx-auto w-full max-w-screen-2xl px-4 sm:px-8 ${OVERLAP}`}>
        <InfoPanel movie={m} />

        <div className="mt-12 space-y-14 pb-16">
          <CastRow castJson={m.cast_json} />
          <MovieFacts movie={m} />

          {/* Watch Party: a mock synced player (no real stream yet) plus live
              chat, both over the page's single watch-party socket. */}
          <section id="watch-party" className="scroll-mt-24" aria-labelledby="watch-party-heading">
            <h2 id="watch-party-heading" className="text-xl font-bold tracking-tight">
              Watch together
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Play, pause and chat in sync with everyone in this movie&apos;s room.
            </p>
            <div className="mt-4 grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,7fr)_minmax(320px,3fr)]">
              <VideoPlayerPlaceholder
                title={m.title}
                backdropPath={m.backdrop_path}
                playback={wp.playback}
                currentUserId={currentUserId}
                connected={wp.status === "open"}
                onPlay={(pos) => wp.sendPlaybackSync("play", pos)}
                onPause={(pos) => wp.sendPlaybackSync("pause", pos)}
                onSeek={(pos) => wp.sendPlaybackSync("seek", pos)}
              />
              {/* On desktop the chat matches the player's height instead of
                  setting its own. */}
              <div className="lg:relative">
                <WatchPartyPanel
                  wp={wp}
                  isAuthed={isAuthed}
                  currentUserId={currentUserId}
                  className="h-[28rem] lg:absolute lg:inset-0 lg:h-auto"
                />
              </div>
            </div>
          </section>

          <section aria-label="Ratings and comments" className="max-w-4xl space-y-8">
            <h2 className="text-xl font-bold tracking-tight">Ratings &amp; comments</h2>
            {interactions.data ? (
              <>
                <RatingSection movie={m} interactions={interactions.data} />
                <CommentSection movieId={movieId} interactions={interactions.data} />
              </>
            ) : (
              <div className="space-y-3">
                <Skeleton className="h-6 w-64 rounded" />
                <Skeleton className="h-20 w-full rounded" />
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}

/** Full-bleed backdrop that dissolves into the page below it. */
function Backdrop({ movie }: { movie: Movie }) {
  const backdrop = backdropUrl(movie.backdrop_path, "original");
  return (
    <section className={`relative w-full overflow-hidden ${HERO}`} aria-hidden>
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

/** Poster + glass panel floating over the bottom edge of the backdrop. */
function InfoPanel({ movie: m }: { movie: Movie }) {
  const poster = posterUrl(m.poster_path, "w500");
  const year = m.release_date?.slice(0, 4);
  const runtime = runtimeLabel(m.runtime);
  const trailerKey = m.trailer_key?.trim();

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
            </span>
          )}
        </div>

        {m.genres && m.genres.length > 0 && (
          <ul className="mt-4 flex flex-wrap gap-2" aria-label="Genres">
            {m.genres.map((g) => (
              <li key={g.id} className="rounded-full border border-white/15 bg-white/5 px-3 py-1 text-xs font-medium text-zinc-200">
                {g.name}
              </li>
            ))}
          </ul>
        )}

        {m.overview && <p className="mt-5 max-w-3xl leading-relaxed text-zinc-200">{m.overview}</p>}

        <div className="mt-6 flex flex-wrap gap-3">
          {trailerKey ? (
            <TrailerDialog
              videoKey={trailerKey}
              title={m.title}
              trigger={
                <Button size="lg" className="h-12 px-6 text-base font-semibold">
                  <Play className="size-5 fill-current" />
                  Play Trailer
                </Button>
              }
            />
          ) : (
            <Button size="lg" className="h-12 px-6 text-base font-semibold" disabled title="TMDB has no YouTube trailer for this title">
              <Play className="size-5" />
              No trailer available
            </Button>
          )}
          <SaveButton movie={m} />
          <CommentSheet
            movieId={m.id}
            title={m.title}
            trigger={
              <Button size="lg" variant="secondary" className={SECONDARY_ACTION}>
                <MessageCircle className="size-5" />
                Comments
              </Button>
            }
          />
          <Button asChild size="lg" variant="secondary" className={SECONDARY_ACTION}>
            <a href="#watch-party">
              <Users className="size-5" />
              Watch together
            </a>
          </Button>
        </div>
      </div>
    </div>
  );
}

const SECONDARY_ACTION = "h-12 bg-white/10 px-6 text-base font-semibold text-white hover:bg-white/20";

/** Add to / remove from My List; opens sign-in when signed out. */
function SaveButton({ movie }: { movie: Movie }) {
  const { saved, toggle } = useSavedToggle(movie);
  return (
    <Button size="lg" variant="secondary" className={SECONDARY_ACTION} onClick={toggle} aria-pressed={saved}>
      {saved ? <BookmarkCheck className="size-5 text-primary" /> : <Bookmark className="size-5" />}
      {saved ? "In My List" : "My List"}
    </Button>
  );
}

function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
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

function MovieDetailSkeleton() {
  return (
    <div className="flex flex-1 flex-col">
      <Skeleton className={`w-full rounded-none ${HERO}`} />
      <div className={`relative z-10 mx-auto w-full max-w-screen-2xl px-4 sm:px-8 ${OVERLAP}`}>
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
