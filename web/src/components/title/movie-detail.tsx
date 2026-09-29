"use client";

import { useEffect, useRef } from "react";
import dynamic from "next/dynamic";
import { useSearchParams } from "next/navigation";
import { MessageCircle, Play, Users } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { CastRow } from "@/components/movies/cast-row";
import { CommentSheet } from "@/components/movies/comment-section";
import { RatingSection } from "@/components/movies/rating-section";
import { MovieFacts } from "@/components/movies/omdb-info";
import { TrailerDialog } from "@/components/movies/trailer-dialog";
import { VideoPlayerPlaceholder } from "@/components/movies/video-player-placeholder";
import { SyncedTrailerPlayer } from "@/components/watch-party/synced-trailer-player";
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
import { useInteractions, useMovieDetails } from "@/hooks/queries";
import { useWatchParty } from "@/hooks/use-watch-party";
import { useT } from "@/i18n";
import { ApiError } from "@/lib/api-client";
import { newPartyCode, parsePartyCode, partyHref, partyRoom } from "@/lib/party";
import { useAuthStore } from "@/store/auth-store";
import type { Movie } from "@/types/movie";

const CommentSection = dynamic(() => import("@/components/movies/comment-section").then((mod) => mod.CommentSection), { ssr: false });
const WatchPartyPanel = dynamic(() => import("@/components/watch-party/watch-party-panel").then((mod) => mod.WatchPartyPanel), { ssr: false });

/**
 * Everything about one movie: backdrop, info, cast, facts, Watch Party,
 * ratings and comments. Rendered by its own page (/movies/[id]) and, on
 * client-side navigation, by the modal over the current page (@modal).
 */
export function MovieDetail({ movieId, variant = "page" }: { movieId: number; variant?: DetailVariant }) {
  const validId = Number.isInteger(movieId) && movieId > 0;
  const { t } = useT();

  const isAuthed = useAuthStore((s) => s.hasHydrated && s.username !== null);
  const currentUserId = useAuthStore((s) => s.userId);

  const movie = useMovieDetails(validId ? movieId : -1);
  const interactions = useInteractions({ id: movieId, media_type: "movie" }, validId);
  // Watch Party: the open room, or the private party in ?party= (an invite
  // link, one just started, or Discover's "Watch Party"). One socket for the
  // whole page: the synced player and the chat panel share it.
  const party = parsePartyCode(useSearchParams().get("party"));
  const room = partyRoom(movieId, party);
  const wp = useWatchParty(room, isAuthed);
  const loaded = movie.status === "success";

  // The URL is the party's state. history.replaceState keeps Next's router
  // (and an intercepted modal) in place while useSearchParams follows it;
  // Next only syncs calls whose state isn't its own, hence null.
  const setParty = (code: string | null) => {
    const url = new URL(window.location.href);
    if (code) url.searchParams.set("party", code);
    else url.searchParams.delete("party");
    window.history.replaceState(null, "", url);
  };

  // Arriving with a party (or starting one): bring it into view, and join
  // it straight away when signed in (once per room).
  const shownFor = useRef<string | null>(null);
  const joinedFor = useRef<string | null>(null);
  const { connect } = wp;
  useEffect(() => {
    if (!party || !loaded) return;
    if (shownFor.current !== room) {
      shownFor.current = room;
      requestAnimationFrame(() => document.getElementById("watch-party")?.scrollIntoView({ behavior: "smooth", block: "start" }));
    }
    if (isAuthed && joinedFor.current !== room) {
      joinedFor.current = room;
      connect();
    }
    // A remount (Strict Mode's rehearsal included) closes the socket: let
    // the next run join again. connect() ignores a socket already open.
    return () => {
      joinedFor.current = null;
    };
  }, [party, loaded, isAuthed, room, connect]);

  if (!validId) {
    return <ErrorState message={t("detail.invalidMovie")} />;
  }

  if (movie.status === "pending") {
    return <TitleDetailSkeleton variant={variant} />;
  }

  if (movie.status === "error") {
    const notFound = movie.error instanceof ApiError && movie.error.status === 404;
    return (
      <ErrorState
        message={t(notFound ? "detail.movieNotFound" : "common.catalogUnreachable")}
        onRetry={notFound ? undefined : () => movie.refetch()}
      />
    );
  }

  const m = movie.data;

  return (
    <div className="flex flex-1 flex-col">
      <TitleBackdrop movie={m} variant={variant} />

      <div className={detailBodyClass(variant)}>
        <TitleInfoPanel movie={m} actions={<MovieActions movie={m} />} />

        <div className="mt-12 space-y-14 pb-16">
          <CastRow castJson={m.cast_json} />
          <MovieFacts movie={m} />

          {/* Watch Party: the trailer, played in sync (a mock player when the
              movie has none), plus live chat, over the page's single socket. */}
          <section id="watch-party" className="scroll-mt-24" aria-labelledby="watch-party-heading">
            <h2 id="watch-party-heading" className="text-xl font-bold tracking-tight">
              {t("watchParty.heading")}
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {t(party ? "watchParty.subtitlePrivate" : "watchParty.subtitleOpen")}
            </p>
            <div className="mt-4 grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,7fr)_minmax(320px,3fr)]">
              {m.trailer_key ? (
                <SyncedTrailerPlayer
                  videoKey={m.trailer_key}
                  title={m.title}
                  playback={wp.playback}
                  currentUserId={currentUserId}
                  connected={wp.status === "open"}
                  onPlay={(pos) => wp.sendPlaybackSync("play", pos)}
                  onPause={(pos) => wp.sendPlaybackSync("pause", pos)}
                />
              ) : (
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
              )}
              {/* On desktop the chat matches the player's height instead of
                  setting its own. */}
              <div className="lg:relative">
                <WatchPartyPanel
                  wp={wp}
                  isAuthed={isAuthed}
                  currentUserId={currentUserId}
                  party={party}
                  invitePath={party ? partyHref(movieId, party) : null}
                  onStartParty={() => setParty(newPartyCode())}
                  onLeaveParty={() => setParty(null)}
                  className="h-[28rem] lg:absolute lg:inset-0 lg:h-auto"
                />
              </div>
            </div>
          </section>

          <section aria-label={t("detail.ratingsAndComments")} className="max-w-4xl space-y-8">
            <h2 className="text-xl font-bold tracking-tight">{t("detail.ratingsAndComments")}</h2>
            {interactions.data ? (
              <>
                <RatingSection movie={m} interactions={interactions.data} />
                <CommentSection subject={m} interactions={interactions.data} />
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

/** Movie actions: trailer, My List, comments, watch party. */
function MovieActions({ movie: m }: { movie: Movie }) {
  const { t } = useT();
  const trailerKey = m.trailer_key?.trim();
  return (
    <>
      {trailerKey ? (
        <TrailerDialog
          videoKey={trailerKey}
          title={m.title}
          trigger={
            <Button size="lg" className="h-12 px-6 text-base font-semibold">
              <Play className="size-5 fill-current" />
              {t("detail.playTrailer")}
            </Button>
          }
        />
      ) : (
        <Button size="lg" className="h-12 px-6 text-base font-semibold" disabled title={t("detail.noTrailerTitle")}>
          <Play className="size-5" />
          {t("detail.noTrailer")}
        </Button>
      )}
      <SaveButton movie={m} />
      <WatchedButton movie={m} />
      <CommentSheet
        subject={m}
        title={m.title}
        trigger={
          <Button size="lg" variant="secondary" className={SECONDARY_ACTION}>
            <MessageCircle className="size-5" />
            {t("detail.comments")}
          </Button>
        }
      />
      <Button asChild size="lg" variant="secondary" className={SECONDARY_ACTION}>
        <a
          href="#watch-party"
          // Scrolled in script: a hash change would add a history entry the
          // router doesn't know, and the modal closes by going back.
          onClick={(e) => {
            e.preventDefault();
            document.getElementById("watch-party")?.scrollIntoView({ behavior: "smooth", block: "start" });
          }}
        >
          <Users className="size-5" />
          {t("detail.watchTogether")}
        </a>
      </Button>
    </>
  );
}
