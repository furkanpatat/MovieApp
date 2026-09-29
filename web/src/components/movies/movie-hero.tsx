"use client";

import { memo, useEffect, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { AnimatePresence, motion } from "framer-motion";
import { Info } from "lucide-react";

import { Button } from "@/components/ui/button";
import { RatingBadge } from "@/components/movies/rating-badge";
import { backdropUrl } from "@/lib/tmdb-image";
import { TVBadge } from "@/components/movies/movie-card";
import { useT } from "@/i18n";
import { isTV, titleHref } from "@/lib/media";
import type { Movie } from "@/types/movie";

const ROTATE_MS = 7000;

/**
 * Full-bleed hero carousel over the top trending titles, crossfading every
 * 7s. A crisp backdrop with layered scrims (not a literal CSS blur) is the
 * actual Netflix/Letterboxd hero pattern — a blurred backdrop reads as a
 * low-res placeholder at this size, where a dark gradient keeps the same
 * legibility without muddying the art.
 *
 * Rotation pauses while the pointer is over the hero (someone reading or
 * about to click) and while the tab is hidden. The timer is keyed on the
 * current slide, so picking a dot also restarts the countdown.
 */
export const MovieHero = memo(function MovieHero({ movies }: { movies: Movie[] }) {
  const { t } = useT();
  const [index, setIndex] = useState(0);
  const [hovered, setHovered] = useState(false);
  const [tabHidden, setTabHidden] = useState(false);
  const count = movies.length;
  const movie = movies[index % count];

  useEffect(() => {
    const onVisibility = () => setTabHidden(document.visibilityState === "hidden");
    document.addEventListener("visibilitychange", onVisibility);
    return () => document.removeEventListener("visibilitychange", onVisibility);
  }, []);

  useEffect(() => {
    if (hovered || tabHidden || count < 2) return;
    const id = setTimeout(() => setIndex((i) => (i + 1) % count), ROTATE_MS);
    return () => clearTimeout(id);
  }, [index, hovered, tabHidden, count]);

  if (!movie) return null;

  return (
    <section
      className="relative h-[56vh] min-h-[420px] w-full overflow-hidden sm:h-[64vh]"
      aria-roledescription="carousel"
      aria-label={t("home.heroLabel")}
      onPointerEnter={(e) => e.pointerType === "mouse" && setHovered(true)}
      onPointerLeave={(e) => e.pointerType === "mouse" && setHovered(false)}
    >
      {/* Slides are stacked and crossfade: the outgoing one fades out while
          the incoming one fades in over it. */}
      <AnimatePresence initial={false}>
        <motion.div
          key={movie.id}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 1, ease: "easeInOut" }}
          className="absolute inset-0"
          aria-roledescription="slide"
          aria-label={t("home.heroSlide", { i: (index % count) + 1, n: count, title: movie.title })}
        >
          <HeroSlide movie={movie} priority={index === 0} />
        </motion.div>
      </AnimatePresence>

      {count > 1 && (
        <div className="absolute inset-x-0 bottom-4 z-10 flex justify-center gap-2 sm:bottom-6">
          {movies.map((m, i) => {
            const active = i === index % count;
            return (
              <button
                key={m.id}
                type="button"
                aria-label={t("home.showSlide", { title: m.title })}
                aria-current={active}
                onClick={() => setIndex(i)}
                className="group flex h-6 items-center px-1 outline-none"
              >
                <span
                  className={`block h-1.5 rounded-full transition-all duration-300 group-focus-visible:ring-2 group-focus-visible:ring-primary ${
                    active ? "w-8 bg-primary" : "w-1.5 bg-white/40 group-hover:bg-white/70"
                  }`}
                />
              </button>
            );
          })}
        </div>
      )}
    </section>
  );
});

function HeroSlide({ movie, priority }: { movie: Movie; priority: boolean }) {
  const { t } = useT();
  const backdrop = backdropUrl(movie.backdrop_path, "original");
  const year = movie.release_date?.slice(0, 4);

  return (
    <>
      {backdrop && (
        <Image src={backdrop} alt="" fill priority={priority} sizes="100vw" className="object-cover" />
      )}

      {/* Scrims: full-bleed dark base so text is legible even without a
          backdrop image, a top scrim so the nav stays readable, and a
          left-to-right scrim for the text block itself. */}
      <div className="absolute inset-0 bg-background/40" />
      <div className="scrim-top absolute inset-x-0 top-0 h-32" />
      <div className="scrim-bottom absolute inset-x-0 bottom-0 h-2/3" />
      <div className="absolute inset-0 bg-gradient-to-r from-background via-background/40 to-transparent sm:via-background/20" />

      <div className="relative flex h-full max-w-screen-2xl flex-col justify-end gap-4 px-4 pb-14 sm:px-8 sm:pb-20">
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6, delay: 0.3, ease: "easeOut" }}
          className="max-w-xl"
        >
          {isTV(movie) && <TVBadge className="mb-3" />}
          <h1 className="text-3xl font-bold tracking-tight text-balance sm:text-5xl">{movie.title}</h1>

          <div className="mt-3 flex items-center gap-3 text-sm text-muted-foreground">
            <RatingBadge movie={movie} size="md" />
            {year && <span>{year}</span>}
          </div>

          {movie.overview && (
            <p className="mt-4 line-clamp-3 text-sm text-muted-foreground sm:text-base">{movie.overview}</p>
          )}

          <div className="mt-6 flex gap-3">
            <Button asChild size="lg" className="font-semibold">
              <Link href={titleHref(movie)}>
                <Info className="size-4" />
                {t("home.moreInfo")}
              </Link>
            </Button>
          </div>
        </motion.div>
      </div>
    </>
  );
}
