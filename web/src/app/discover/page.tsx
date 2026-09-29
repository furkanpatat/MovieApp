"use client";

import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import Link from "next/link";
import { ActionRail } from "@/components/discover/action-rail";
import { FeedPlayer } from "@/components/discover/feed-player";
import Image from "next/image";
import { motion, AnimatePresence, useInView } from "framer-motion";
import { Heart } from "lucide-react";

import { useDiscover, useRateMovie, useInteractions, useTitleDetails } from "@/hooks/queries";
import { titleHref, titleKey } from "@/lib/media";
import { useT } from "@/i18n";
import { useMediaMode } from "@/store/media-mode-store";
import { GenrePicker } from "@/components/discover/genre-picker";
import { backdropUrl, posterUrl } from "@/lib/tmdb-image";
import type { Movie } from "@/types/movie";

import { useFeedStore } from "@/store/feed-store";
import { useTitleModalStore } from "@/store/modal-store";
import { useMyRating, useRequireAuth, useSavedToggle } from "@/hooks/use-library";
import { RatingBadge } from "@/components/movies/rating-badge";

import { memo } from "react";

const DiscoverPost = memo(function DiscoverPost({ 
  movie, 
  index,
  totalMovies,
  hasNextPage,
  fetchNextPage
}: { 
  movie: Movie; 
  index: number;
  totalMovies: number;
  hasNextPage: boolean;
  fetchNextPage: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  // A title opened in the modal covers the feed: pause as if scrolled away.
  const modalOpen = useTitleModalStore((s) => s.open);
  const isInView = useInView(ref, { amount: 0.6 }) && !modalOpen;

  const [showHeart, setShowHeart] = useState(false);
  const [isPlaying, setIsPlaying] = useState(true);
  
  const { isMuted, toggleMute, setMuted } = useFeedStore();
  
  const rateMutation = useRateMovie(movie);
  const myRating = useMyRating(movie);
  const requireAuth = useRequireAuth();
  const { saved, toggle: toggleSaved } = useSavedToggle(movie);
  // Only the post on screen: the feed renders 20 posts per page.
  const { data: interactions } = useInteractions(movie, isInView);
  const { data: details } = useTitleDetails(movie, isInView);

  const videoKey = details?.trailer_key;
  const overview = details?.overview || movie.overview;

  // "Liked" is the user's own rating (a like is a 10), not the global average.
  const isLiked = (myRating ?? 0) >= 8 || rateMutation.isPending;
  const like = () => requireAuth(() => rateMutation.mutate(10));

  useEffect(() => {
    if (isInView && index >= totalMovies - 2 && hasNextPage) fetchNextPage();
  }, [isInView, index, totalMovies, hasNextPage, fetchNextPage]);


  const lastTapRef = useRef<number>(0);
  const tapTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  const handleTap = (e: React.TouchEvent | React.MouseEvent) => {
    const now = Date.now();
    const DOUBLE_TAP_DELAY = 300;
    
    if (now - lastTapRef.current < DOUBLE_TAP_DELAY) {
      if (tapTimeoutRef.current) {
        clearTimeout(tapTimeoutRef.current);
        tapTimeoutRef.current = null;
      }
      requireAuth(() => {
        setShowHeart(true);
        rateMutation.mutate(10);
        setTimeout(() => setShowHeart(false), 800);
      });
    } else {
      tapTimeoutRef.current = setTimeout(() => {
        setIsPlaying((playing) => !playing);
      }, DOUBLE_TAP_DELAY);
    }
    
    lastTapRef.current = now;
  };


  const backdrop = backdropUrl(movie.backdrop_path, "original") || posterUrl(movie.poster_path, "w500");

  return (
    <div ref={ref} className="relative h-full w-full overflow-hidden bg-zinc-950 transform-gpu will-change-transform">
      <div className="absolute inset-0">
        {videoKey && isInView ? (
          <div className="absolute inset-0 pointer-events-none">
            <FeedPlayer videoKey={videoKey} playing={isPlaying} muted={isMuted} onSoundBlocked={() => setMuted(true)} />
          </div>
        ) : (
          backdrop && (
            <Image
              src={backdrop}
              alt={movie.title}
              fill
              className="object-cover opacity-60"
              priority={isInView}
            />
          )
        )}
      </div>

      <div className="absolute inset-0 bg-gradient-to-b from-transparent via-zinc-950/20 to-zinc-950/90 pointer-events-none" />

      <div
        className="absolute inset-0 z-10 cursor-pointer"
        onClick={handleTap}
      />

      <AnimatePresence>
        {showHeart && (
          <motion.div
            initial={{ scale: 0.5, opacity: 0 }}
            animate={{ scale: 1.5, opacity: 1 }}
            exit={{ scale: 2, opacity: 0 }}
            transition={{ duration: 0.5, ease: "easeOut" }}
            className="pointer-events-none absolute left-1/2 top-1/2 z-20 -translate-x-1/2 -translate-y-1/2 text-primary drop-shadow-2xl"
          >
            <Heart className="size-32 fill-current" />
          </motion.div>
        )}
      </AnimatePresence>



      <div className="absolute bottom-0 left-0 z-20 w-3/4 p-4 pb-6 sm:p-6 sm:pb-8 pointer-events-none">
        <h2 className="text-2xl font-bold text-white sm:text-3xl drop-shadow-md">
          <Link href={titleHref(movie)} className="pointer-events-auto hover:underline underline-offset-4">
            {movie.title}
          </Link>
        </h2>
        <div className="mt-2 flex items-center gap-2 text-sm text-zinc-300">
          <RatingBadge movie={details ?? movie} size="md" />
          <span>•</span>
          <span>{movie.release_date?.slice(0, 4)}</span>
        </div>
        <p className="mt-2 line-clamp-3 text-sm text-zinc-300 drop-shadow-sm">
          {overview}
        </p>
      </div>

      <ActionRail
        movie={movie}
        muted={isMuted}
        onToggleMute={toggleMute}
        liked={isLiked}
        likes={interactions?.total_votes ?? movie.vote_count ?? 0}
        onLike={like}
        comments={interactions?.recent_comments?.length ?? 0}
        saved={saved}
        onToggleSaved={toggleSaved}
      />
    </div>
  );
});

export default function DiscoverPage() {
  const { t } = useT();
  // Movies or series (the logo switch), each with its own genre filter.
  const { mode, ready } = useMediaMode();
  const genreId = useFeedStore((s) => s.genres[mode] ?? 0);
  const setGenre = useFeedStore((s) => s.setGenre);
  // A new random start on every visit (and every genre switch).
  const [seed] = useState(() => Math.floor(Math.random() * 2 ** 31));
  // Wait for the saved genre (localStorage) rather than fetch "all" first.
  const hydrated = useSyncExternalStore(
    (onChange) => useFeedStore.persist.onFinishHydration(onChange),
    () => useFeedStore.persist.hasHydrated(),
    () => false,
  );
  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isPending, isError, refetch } = useDiscover(
    mode,
    genreId,
    seed,
    hydrated && ready,
  );

  // Pages overlap as popularity shifts; the feed is keyed by title.
  const movies = useMemo(() => {
    const seen = new Set<string>();
    return (data?.pages ?? []).flatMap((p) => p.results).filter((m) => !seen.has(titleKey(m)) && seen.add(titleKey(m)));
  }, [data]);

  // A start page past a small genre's end comes back empty: move on.
  useEffect(() => {
    if (data && movies.length === 0 && hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [data, movies.length, hasNextPage, isFetchingNextPage, fetchNextPage]);

  return (
    <div className="relative h-dvh w-full bg-zinc-950">
      <GenrePicker mode={mode} value={genreId} onChange={(id) => setGenre(mode, id)} />

      {isPending || (movies.length === 0 && hasNextPage) ? (
        <div className="flex h-full w-full items-center justify-center">
          <div className="size-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        </div>
      ) : isError || movies.length === 0 ? (
        <div className="flex h-full flex-col items-center justify-center gap-4 px-6 text-center">
          <p className="text-lg font-semibold">{t("discover.loadFailed")}</p>
          <button type="button" onClick={() => refetch()} className="rounded-full bg-white/10 px-5 py-2 text-sm font-semibold hover:bg-white/20">
            {t("common.tryAgain")}
          </button>
        </div>
      ) : (
        // Keyed by mode and genre: a new filter starts at the top of its own feed.
        <div key={`${mode}-${genreId}`} className="hide-scrollbar h-dvh w-full snap-y snap-mandatory overflow-y-scroll scroll-smooth transform-gpu will-change-transform">
          {movies.map((movie, idx) => (
            <div key={titleKey(movie)} className="snap-center h-dvh w-full snap-always">
              <DiscoverPost
                movie={movie}
                index={idx}
                totalMovies={movies.length}
                hasNextPage={hasNextPage}
                fetchNextPage={fetchNextPage}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
