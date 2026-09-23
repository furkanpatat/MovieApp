"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { motion, AnimatePresence, useInView } from "framer-motion";
import { Bookmark, BookmarkCheck, Heart, MessageCircle, Share2, Users, Volume2, VolumeX } from "lucide-react";
import { toast } from "sonner";

import { usePopularMovies, useRateMovie, useInteractions, useMovieDetails } from "@/hooks/queries";
import { backdropUrl, posterUrl } from "@/lib/tmdb-image";
import type { Movie } from "@/types/movie";

import { useFeedStore } from "@/store/feed-store";
import { CommentSheet } from "@/components/movies/comment-section";
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
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const isInView = useInView(ref, { amount: 0.6 });

  const [showHeart, setShowHeart] = useState(false);
  const [isPlaying, setIsPlaying] = useState(true);
  
  const { isMuted, toggleMute } = useFeedStore();
  
  const rateMutation = useRateMovie(movie.id, movie);
  const myRating = useMyRating(movie.id);
  const requireAuth = useRequireAuth();
  const { saved, toggle: toggleSaved } = useSavedToggle(movie);
  const { data: interactions } = useInteractions(movie.id);
  const { data: details } = useMovieDetails(movie.id, isInView);

  const videoKey = details?.trailer_key;
  const overview = details?.overview || movie.overview;

  // "Liked" is the user's own rating (a like is a 10), not the global average.
  const isLiked = (myRating ?? 0) >= 8 || rateMutation.isPending;
  const like = () => requireAuth(() => rateMutation.mutate(10));

  // YouTube IFrame API command over postMessage (the player UI is hidden).
  const sendCommand = (command: string) => {
    if (iframeRef.current?.contentWindow) {
      iframeRef.current.contentWindow.postMessage(
        JSON.stringify({ event: "command", func: command }),
        "*"
      );
    }
  };

  useEffect(() => {
    if (isInView) {
      if (index >= totalMovies - 2 && hasNextPage) {
        fetchNextPage();
      }
      if (iframeRef.current && isPlaying) {
        sendCommand("playVideo");
      }
    } else {
      if (iframeRef.current) {
        sendCommand("pauseVideo");
      }
    }
  }, [isInView, isPlaying, index, totalMovies, hasNextPage, fetchNextPage]);

  useEffect(() => {
    if (isInView && iframeRef.current) {
      sendCommand(isMuted ? "mute" : "unMute");
    }
  }, [isMuted, isInView]);


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
        const nextPlaying = !isPlaying;
        setIsPlaying(nextPlaying);
        sendCommand(nextPlaying ? "playVideo" : "pauseVideo");
      }, DOUBLE_TAP_DELAY);
    }
    
    lastTapRef.current = now;
  };

  const handleToggleMute = (e: React.MouseEvent) => {
    e.stopPropagation();
    toggleMute();
  };

  const backdrop = backdropUrl(movie.backdrop_path, "original") || posterUrl(movie.poster_path, "w500");

  return (
    <div ref={ref} className="relative h-full w-full overflow-hidden bg-zinc-950 transform-gpu will-change-transform">
      <div className="absolute inset-0">
        {videoKey && isInView ? (
          <div className="absolute inset-0 pointer-events-none">
            <iframe
              ref={iframeRef}
              onLoad={() => sendCommand(isMuted ? "mute" : "unMute")}
              src={`https://www.youtube.com/embed/${videoKey}?autoplay=1&mute=1&controls=0&disablekb=1&fs=0&modestbranding=1&rel=0&playsinline=1&enablejsapi=1&loop=1&playlist=${videoKey}`}
              allow="autoplay"
              className="absolute left-1/2 top-1/2 h-[300%] w-[300%] -translate-x-1/2 -translate-y-1/2 sm:h-[150%] sm:w-[150%] object-cover pointer-events-none"
            />
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
          {movie.title}
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

      <div className="absolute bottom-6 right-4 z-20 flex flex-col items-center gap-5 sm:bottom-8 sm:right-6">
        <div className="flex flex-col items-center gap-1">
          <button
            aria-label="Toggle Mute"
            className="group rounded-full bg-black/40 p-3 text-white backdrop-blur-md transition hover:bg-black/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            onClick={handleToggleMute}
          >
            {isMuted ? (
              <VolumeX className="size-7 transition-transform group-hover:scale-110" />
            ) : (
              <Volume2 className="size-7 transition-transform group-hover:scale-110" />
            )}
          </button>
        </div>

        <div className="flex flex-col items-center gap-1">
          <button
            aria-label="Like"
            className="group rounded-full bg-black/40 p-3 text-white backdrop-blur-md transition hover:bg-black/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            onClick={(e) => {
              e.stopPropagation();
              like();
            }}
          >
            <Heart className={`size-7 transition-transform group-hover:scale-110 ${isLiked ? 'fill-primary text-primary' : ''}`} />
          </button>
          <span className="text-xs font-medium text-white drop-shadow-sm">
            {interactions?.total_votes ?? movie.vote_count ?? 0}
          </span>
        </div>

        <div className="flex flex-col items-center gap-1">
          <CommentSheet
            movieId={movie.id}
            title={movie.title}
            trigger={
              <button
                aria-label="Comments"
                className="group rounded-full bg-black/40 p-3 text-white backdrop-blur-md transition hover:bg-black/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              >
                <MessageCircle className="size-7 transition-transform group-hover:scale-110" />
              </button>
            }
          />
          <span className="text-xs font-medium text-white drop-shadow-sm">
            {interactions?.recent_comments?.length ?? 0}
          </span>
        </div>

        <div className="flex flex-col items-center gap-1">
          <button
            aria-label={saved ? "Remove from My List" : "Add to My List"}
            aria-pressed={saved}
            onClick={(e) => {
              e.stopPropagation();
              toggleSaved();
            }}
            className="group rounded-full bg-black/40 p-3 text-white backdrop-blur-md transition hover:bg-black/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          >
            {saved ? (
              <BookmarkCheck className="size-7 text-primary transition-transform group-hover:scale-110" />
            ) : (
              <Bookmark className="size-7 transition-transform group-hover:scale-110" />
            )}
          </button>
          <span className="text-xs font-medium text-white drop-shadow-sm">{saved ? "Saved" : "Save"}</span>
        </div>

        <div className="flex flex-col items-center gap-1">
          {/* No /party route yet; Watch Party is a placeholder until it ships. */}
          <button
            type="button"
            aria-label="Watch Party"
            onClick={() => toast("🍿 Watch Party feature is coming soon!", { id: "watch-party-soon" })}
            className="group rounded-full bg-black/40 p-3 text-white backdrop-blur-md transition hover:bg-black/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          >
            <Users className="size-7 transition-transform group-hover:scale-110" />
          </button>
          <span className="text-xs font-medium text-white drop-shadow-sm">Party</span>
        </div>

        <div className="flex flex-col items-center gap-1">
          <button
            aria-label="Share"
            className="group rounded-full bg-black/40 p-3 text-white backdrop-blur-md transition hover:bg-black/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          >
            <Share2 className="size-7 transition-transform group-hover:scale-110" />
          </button>
          <span className="text-xs font-medium text-white drop-shadow-sm">Share</span>
        </div>
      </div>
    </div>
  );
});

export default function DiscoverPage() {
  const { data, fetchNextPage, hasNextPage, isLoading } = usePopularMovies();
  const movies = data?.pages.flatMap((p) => p.results) ?? [];

  if (isLoading) {
    return (
      <div className="flex h-dvh w-full items-center justify-center bg-zinc-950">
        <div className="size-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
      </div>
    );
  }

  if (!movies.length) return null;

  return (
    <div
      className="h-dvh w-full snap-y snap-mandatory overflow-y-scroll bg-zinc-950 scroll-smooth transform-gpu will-change-transform"
      style={{ scrollbarWidth: 'none', msOverflowStyle: 'none' }}
    >
      <style dangerouslySetInnerHTML={{ __html: `
        ::-webkit-scrollbar {
          display: none;
        }
      `}} />
      
      {movies.map((movie, idx) => {
        const isNearEnd = idx >= movies.length - 2;
        return (
          <div key={`${movie.id}-${idx}`} className="snap-center h-dvh w-full snap-always">
            <DiscoverPost 
              movie={movie} 
              index={idx}
              totalMovies={movies.length}
              hasNextPage={hasNextPage}
              fetchNextPage={fetchNextPage}
            />
          </div>
        );
      })}
    </div>
  );
}
