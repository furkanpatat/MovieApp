"use client";

import { memo, useState, useRef, useEffect, useLayoutEffect, useCallback, type PointerEvent, type MouseEvent } from "react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { motion, AnimatePresence } from "framer-motion";
import { Clapperboard, Play, Info, Tv, X } from "lucide-react";

import { posterUrl, backdropUrl } from "@/lib/tmdb-image";
import { RatingBadge } from "@/components/movies/rating-badge";
import type { Movie } from "@/types/movie";
import { useTitleDetails } from "@/hooks/queries";
import { useMediaQuery } from "@/hooks/use-media-query";
import { currentLocale, plural, translate, useT } from "@/i18n";
import { isTV, titleHref } from "@/lib/media";
import { useTitleModalStore } from "@/store/modal-store";

const EXPAND_MAX_HEIGHT = 550;
const VIEWPORT_MARGIN = 16;
const HOVER_INTENT_MS = 800;

const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi);

/** The usable viewport: below the sticky site header (z-50), inside a margin. */
function viewportBounds() {
  const vw = document.documentElement.clientWidth; // excludes the scrollbar
  const vh = window.innerHeight;
  const headerBottom = Math.max(0, document.querySelector("header")?.getBoundingClientRect().bottom ?? 0);
  const minTop = headerBottom + VIEWPORT_MARGIN;
  return { vw, vh, minTop, maxHeight: vh - minTop - VIEWPORT_MARGIN };
}

interface Placement {
  top: number;
  left: number;
  transformOrigin: string;
}

/**
 * Where a width x height panel goes, in viewport coordinates: centred on the
 * card, then pushed back inside the viewport. So a card near the bottom grows
 * upwards, one near the right edge grows leftwards, etc.
 *
 * Expanded panels are `position: fixed` at these coordinates rather than
 * absolute inside the card: cards live in horizontal scroll rows, and an
 * overflow-x:auto box always clips vertically too (overflow-y:visible
 * computes to auto), so an absolute panel would be cut to the row's height.
 * A fixed box escapes that clipping as long as no ancestor has a transform,
 * filter or will-change: transform.
 */
function placePanel(card: DOMRect, width: number, height: number): Placement {
  const { vw, vh, minTop } = viewportBounds();
  const left = clamp(
    (card.width - width) / 2,
    VIEWPORT_MARGIN - card.left,
    vw - VIEWPORT_MARGIN - card.left - width,
  );
  const top = clamp((card.height - height) / 2, minTop - card.top, vh - VIEWPORT_MARGIN - card.top - height);

  // Grow out of the card: anchor the origin on the card's centre.
  const ox = ((card.width / 2 - left) / width) * 100;
  const oy = ((card.height / 2 - top) / height) * 100;
  return { top: card.top + top, left: card.left + left, transformOrigin: `${ox}% ${oy}%` };
}

interface DesktopBox extends Placement {
  width: number;
  height: number;
  /** Shrinking back onto the card; still fixed until the animation lands. */
  closing?: boolean;
}

// Upper bound on the collapse morph, in case its completion callback never
// fires (e.g. there was nothing to animate).
const COLLAPSE_FALLBACK_MS = 900;

function computeDesktopBox(card: DOMRect): DesktopBox {
  const { vw, maxHeight } = viewportBounds();
  const width = Math.min(vw * 0.9, vw >= 1280 ? 1100 : 1000);
  const height = Math.min(EXPAND_MAX_HEIGHT, maxHeight);
  return { ...placePanel(card, width, height), width, height };
}

/** Mobile panel: 90vw wide, height is content-driven up to 80vh. */
function mobilePanelSize() {
  const { vw, vh, maxHeight } = viewportBounds();
  return { width: vw * 0.9, maxHeight: Math.min(vh * 0.8, maxHeight) };
}

/** Trailer (muted autoplay, YouTube chrome shielded) with a backdrop fallback. */
function TrailerMedia({
  isLoading,
  videoKey,
  backdrop,
  title,
  iframeClassName,
}: {
  isLoading: boolean;
  videoKey?: string;
  backdrop?: string | null;
  title: string;
  iframeClassName: string;
}) {
  return (
    <>
      {isLoading ? (
        <div className="absolute inset-0 bg-zinc-900 animate-pulse" />
      ) : videoKey ? (
        <div className="absolute inset-0 pointer-events-none">
          <iframe
            key={videoKey}
            src={`https://www.youtube.com/embed/${videoKey}?autoplay=1&mute=1&controls=0&playsinline=1`}
            allow="autoplay; encrypted-media"
            className={`absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 object-cover ${iframeClassName}`}
          />
        </div>
      ) : (
        backdrop && <Image src={backdrop} alt={title} fill className="object-cover opacity-90" />
      )}
      {/* Seamless Gradient Mask vertically to blend into Info panel */}
      <div className="absolute inset-0 bg-gradient-to-t from-zinc-950 via-transparent to-transparent pointer-events-none" />
    </>
  );
}

/** Title, meta row, overview and the Play / More Info actions. */
function ExpandedInfo({
  movie,
  details,
  year,
  overview,
  overviewClassName,
}: {
  movie: Movie;
  details?: Movie;
  year?: string;
  overview: string;
  overviewClassName: string;
}) {
  const { t } = useT();
  return (
    <>
      <div>
        <Link href={titleHref(movie)} className="block outline-none mb-2">
          <h3 className="text-2xl md:text-3xl font-bold text-white leading-tight drop-shadow-md line-clamp-1">
            {movie.title}
          </h3>
        </Link>

        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm font-semibold mb-3">
          <span className="text-green-400">{movie.vote_average > 0 ? t("card.match", { n: (movie.vote_average * 10).toFixed(0) }) : t("card.new")}</span>
          <RatingBadge movie={details ?? movie} size="sm" />
          {year && <span className="text-zinc-300">{year}</span>}
          {isTV(movie) && <span className="text-zinc-300">{seasonsLabel(details) ?? t("card.tvSeries")}</span>}
          <span className="border border-zinc-600 px-1.5 rounded text-[10px] text-zinc-300">HD</span>
        </div>

        <p className={`text-sm text-zinc-400 mb-2 ${overviewClassName}`}>{overview}</p>
      </div>

      <div className="flex items-center gap-3">
        <Link href={titleHref(movie)} className="flex-1">
          <button className="flex w-full items-center justify-center gap-2 rounded bg-white py-2 md:py-2.5 text-sm font-bold text-black hover:bg-zinc-200 transition shadow-md">
            <Play className="size-5 fill-black" />
            {t("card.play")}
          </button>
        </Link>
        <Link href={titleHref(movie)} className="flex-1">
          <button className="flex w-full items-center justify-center gap-2 rounded bg-zinc-800 py-2 md:py-2.5 text-sm font-bold text-white hover:bg-zinc-700 transition border-none shadow-md">
            <Info className="size-5 text-white" />
            {t("card.moreInfo")}
          </button>
        </Link>
      </div>
    </>
  );
}

/** "8 Seasons" once a series' details are loaded. */
export function seasonsLabel(show?: Movie) {
  const n = show?.number_of_seasons;
  return n ? translate(currentLocale(), plural(n, "card.seasonsOne", "card.seasonsOther"), { n }) : null;
}

/** A small glass tag marking a series among movies. */
export function TVBadge({ className = "" }: { className?: string }) {
  const { t } = useT();
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-md bg-black/55 px-1.5 py-0.5 text-[10px] font-semibold tracking-wider text-white uppercase ring-1 ring-white/15 backdrop-blur-md ${className}`}
    >
      <Tv className="size-3" strokeWidth={2.25} />
      {t("card.tvSeries")}
    </span>
  );
}

function CloseButton({ onClose }: { onClose: () => void }) {
  const { t } = useT();
  return (
    <button
      type="button"
      aria-label={t("card.closePreview")}
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        onClose();
      }}
      className="absolute right-2 top-2 z-10 flex size-9 items-center justify-center rounded-full bg-black/70 text-white backdrop-blur-sm transition hover:bg-black/90"
    >
      <X className="size-5" />
    </button>
  );
}

export const MovieCard = memo(function MovieCard({ movie }: { movie: Movie }) {
  const poster = posterUrl(movie.poster_path, "w500");
  const backdrop = backdropUrl(movie.backdrop_path, "w1280") || poster;
  const year = movie.release_date?.slice(0, 4);

  // Layout follows the md breakpoint; interaction follows the input device
  // (a touch tablet at md+ gets the horizontal layout, opened by tap).
  const isDesktop = useMediaQuery("(min-width: 768px)");

  const [isHovered, setIsHovered] = useState(false);
  // Which layout the card was opened in. Crossing the breakpoint while open
  // (rotation, resize) makes it stale, which simply renders as collapsed.
  const [expandedMode, setExpandedMode] = useState<"desktop" | "mobile" | null>(null);
  const [openedByTouch, setOpenedByTouch] = useState(false);
  const [originClass, setOriginClass] = useState("origin-center");
  const [desktopBox, setDesktopBox] = useState<DesktopBox | null>(null);
  const [mobileLayout, setMobileLayout] = useState<(Placement & { width: number; maxHeight: number }) | null>(null);

  const timerRef = useRef<NodeJS.Timeout | null>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const cardRef = useRef<HTMLDivElement>(null);
  const mobilePanelRef = useRef<HTMLDivElement>(null);
  const lastPointerType = useRef<string>("mouse");

  const isDesktopExpanded = expandedMode === "desktop" && isDesktop && desktopBox !== null && !desktopBox.closing;
  // The collapse must also run while fixed: switching straight back to
  // absolute would put the still-large panel inside the row's scroller, which
  // clips it (overflow-x:auto clips vertically too) for the whole shrink.
  const isDesktopClosing = desktopBox?.closing === true;
  const isFixed = isDesktopExpanded || isDesktopClosing;
  const isMobileExpanded = expandedMode === "mobile" && !isDesktop;
  const isExpanded = isDesktopExpanded || isMobileExpanded;

  const { data: details, isLoading } = useTitleDetails(movie, isExpanded);
  const videoKey = details?.trailer_key;
  const overview = details?.overview || movie.overview;

  const clearTimer = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const expand = (byTouch: boolean) => {
    // Read the breakpoint and measure now, not on enter: the page may have
    // scrolled or been resized during the hover delay.
    const desktop = window.matchMedia("(min-width: 768px)").matches;
    if (desktop && cardRef.current) {
      setDesktopBox(computeDesktopBox(cardRef.current.getBoundingClientRect()));
    }
    setOpenedByTouch(byTouch);
    setExpandedMode(desktop ? "desktop" : "mobile");
  };

  const collapse = useCallback(() => {
    clearTimer();
    setIsHovered(false);
    setExpandedMode(null);
    setOpenedByTouch(false);
    // Retarget the fixed box onto the card so the panel morphs back into it;
    // it drops to absolute once that lands (see onLayoutAnimationComplete).
    setDesktopBox((box) => {
      if (!box || box.closing || !cardRef.current) return box?.closing ? box : null;
      const r = cardRef.current.getBoundingClientRect();
      return { top: r.top, left: r.left, width: r.width, height: r.height, transformOrigin: box.transformOrigin, closing: true };
    });
    setMobileLayout(null);
    setOriginClass("origin-center");
  }, [clearTimer]);

  // A click anywhere on an open card's panel opens the title (its links and
  // buttons, e.g. close, keep their own behavior). Opening it puts the modal
  // up, which collapses the card (see the modal-store subscription).
  const router = useRouter();
  const openFromPanel = (e: MouseEvent) => {
    if ((e.target as HTMLElement).closest("a, button")) return;
    router.push(titleHref(movie));
  };

  const finishClosing = useCallback(() => setDesktopBox((box) => (box?.closing ? null : box)), []);

  useEffect(() => {
    if (!isDesktopClosing) return;
    const id = setTimeout(finishClosing, COLLAPSE_FALLBACK_MS);
    return () => clearTimeout(id);
  }, [isDesktopClosing, finishClosing]);

  // Hover intent is mouse-only: mobile browsers fire emulated mouseenter on
  // tap, which would otherwise start the timer under a finger.
  const handlePointerEnter = (e: PointerEvent) => {
    if (e.pointerType !== "mouse" || isExpanded) return;
    setIsHovered(true);

    if (cardRef.current) {
      const rect = cardRef.current.getBoundingClientRect();
      const windowWidth = window.innerWidth;
      // Keep the hover scale-up from poking past the screen edge.
      if (isDesktop && rect.left < 300) setOriginClass("origin-left");
      else if (isDesktop && windowWidth - rect.right < 300) setOriginClass("origin-right");
      else setOriginClass("origin-center");
    }

    clearTimer();
    timerRef.current = setTimeout(() => expand(false), HOVER_INTENT_MS);
  };

  const handlePointerLeave = (e: PointerEvent) => {
    // Touch fires pointerleave on finger-up; a tapped card stays open until X.
    if (e.pointerType !== "mouse" || openedByTouch) return;
    collapse();
  };

  // First tap on a touch device opens the preview instead of following the
  // poster link. Capture phase, so it runs before next/link's own onClick.
  const handleClickCapture = (e: MouseEvent) => {
    if (lastPointerType.current === "mouse" || isExpanded) return;
    e.preventDefault();
    e.stopPropagation();
    expand(true);
  };

  useEffect(() => clearTimer, [clearTimer]);

  // Opening a title (this one or another) puts a modal over the row: an
  // expanded preview must not keep playing underneath.
  useEffect(() => useTitleModalStore.subscribe((s) => s.open && collapse()), [collapse]);

  // Panels are fixed to the viewport, so once the page or the card's row
  // scrolls they'd float away from their card: close them instead, as
  // Netflix does. Scrolling inside the panel itself (mobile) doesn't count.
  useEffect(() => {
    if (!isExpanded) return;
    const onScroll = (e: Event) => {
      if (e.target instanceof Node && wrapperRef.current?.contains(e.target)) return;
      collapse();
    };
    window.addEventListener("scroll", onScroll, { capture: true, passive: true });
    return () => window.removeEventListener("scroll", onScroll, { capture: true });
  }, [isExpanded, collapse]);

  // An open card closes on Escape or a click/tap anywhere outside it.
  useEffect(() => {
    if (!isExpanded) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && collapse();
    const onPointerDown = (e: globalThis.PointerEvent) => {
      if (!wrapperRef.current?.contains(e.target as Node)) collapse();
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("pointerdown", onPointerDown);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("pointerdown", onPointerDown);
    };
  }, [isExpanded, collapse]);

  // The mobile panel is h-auto, so its height is only known once rendered:
  // measure before paint, then re-clamp whenever it resizes (e.g. the
  // skeleton swapping for the real trailer and overview).
  useLayoutEffect(() => {
    if (!isMobileExpanded) return;
    const place = () => {
      const panel = mobilePanelRef.current;
      const card = cardRef.current;
      if (!panel || !card) return;
      const { width, maxHeight } = mobilePanelSize();
      const height = Math.min(panel.offsetHeight, maxHeight);
      setMobileLayout({ ...placePanel(card.getBoundingClientRect(), width, height), width, maxHeight });
    };
    place();
    const ro = new ResizeObserver(place);
    if (mobilePanelRef.current) ro.observe(mobilePanelRef.current);
    return () => ro.disconnect();
  }, [isMobileExpanded]);

  return (
    <div
      ref={wrapperRef}
      className="group relative block w-full outline-none focus-visible:ring-2 focus-visible:ring-primary/60 rounded-lg"
      onPointerDown={(e) => (lastPointerType.current = e.pointerType)}
      onPointerEnter={handlePointerEnter}
      onPointerLeave={handlePointerLeave}
      onClickCapture={handleClickCapture}
    >
      {/* Measured for clamping: expanded panels are placed around this box. */}
      <div ref={cardRef} className="relative w-full aspect-2/3">
        {/* The Animated Morphing Card (morphs on desktop only) */}
        <motion.div
          layout={isDesktop}
          animate={{
            zIndex: isExpanded || isDesktopClosing ? 50 : isHovered ? 10 : 1,
            scale: isHovered && !isExpanded ? 1.045 : 1,
          }}
          transition={{ type: "spring", bounce: 0.2, duration: 0.5 }}
          onLayoutAnimationComplete={() => isDesktopClosing && finishClosing()}
          onClick={isDesktopExpanded ? openFromPanel : undefined}
          style={
            isFixed && desktopBox
              ? {
                  top: desktopBox.top,
                  left: desktopBox.left,
                  width: desktopBox.width,
                  height: desktopBox.height,
                  transformOrigin: desktopBox.transformOrigin,
                }
              : undefined
          }
          className={`flex flex-row overflow-hidden rounded-xl bg-zinc-950 shadow-black/80 transform-gpu will-change-transform ${
            isDesktopExpanded
              ? "fixed cursor-pointer shadow-2xl border border-zinc-800/50"
              : isDesktopClosing
                ? "fixed shadow-lg border-transparent"
                : `absolute inset-0 ${originClass} w-full h-full shadow-lg border-transparent`
          }`}
        >
          {/* Left/Main: Poster (Slides from 100% to 30% on desktop) */}
          <motion.div
            layout={isDesktop}
            transition={{ type: "spring", bounce: 0.2, duration: 0.5 }}
            className={`relative shrink-0 overflow-hidden bg-secondary ${
              isDesktopExpanded ? "w-[30%] h-full" : "w-full h-full"
            }`}
          >
            <Link href={titleHref(movie)} className="block w-full h-full outline-none">
              {poster ? (
                <Image
                  src={poster}
                  alt={movie.title}
                  fill
                  sizes="(max-width: 768px) 50vw, (max-width: 1024px) 25vw, 20vw"
                  className="object-cover"
                />
              ) : (
                <div className="flex size-full items-center justify-center bg-secondary">
                  <Clapperboard className="size-10 text-muted-foreground" strokeWidth={1.5} />
                </div>
              )}

              <div className="scrim-bottom absolute inset-0 pointer-events-none" />

              {isTV(movie) && !isDesktopExpanded && <TVBadge className="absolute left-2 top-2" />}

              <AnimatePresence>
                {isHovered && !isExpanded && (
                  <motion.div
                    initial={{ opacity: 0, y: 6 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0 }}
                    className="absolute right-2 top-2 rounded-md bg-black/75 px-1.5 py-1 backdrop-blur-sm"
                  >
                    <RatingBadge movie={details ?? movie} size="sm" />
                  </motion.div>
                )}
              </AnimatePresence>
            </Link>
          </motion.div>

          {/* Desktop right: Video + Info (Fades in on expansion) */}
          <AnimatePresence>
            {isDesktopExpanded && (
              <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                transition={{ delay: 0.2, duration: 0.3 }}
                className="flex flex-col w-[70%] h-full shrink-0 bg-zinc-950 border-l border-zinc-800"
              >
                {/* Top: Media (55% height) */}
                <div className="relative w-full h-[55%] bg-black overflow-hidden shrink-0">
                  <TrailerMedia
                    isLoading={isLoading}
                    videoKey={videoKey}
                    backdrop={backdrop}
                    title={movie.title}
                    iframeClassName="h-[300%] w-[300%] sm:h-[150%] sm:w-[150%]"
                  />
                </div>

                {/* Bottom: Info (45% height) */}
                <div className="w-full h-[45%] p-5 md:p-6 flex flex-col justify-between shrink-0 bg-zinc-950">
                  <ExpandedInfo movie={movie} details={details} year={year} overview={overview} overviewClassName="line-clamp-2 md:line-clamp-3" />
                </div>
              </motion.div>
            )}
          </AnimatePresence>

          {isDesktopExpanded && openedByTouch && <CloseButton onClose={collapse} />}
        </motion.div>

        {/* Mobile: a separate vertical panel that scales/fades in over the
            card — no layout morph, so it stays cheap on phones. */}
        <AnimatePresence>
          {isMobileExpanded && (
            <motion.div
              ref={mobilePanelRef}
              initial={{ opacity: 0, scale: 0.92 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.92 }}
              transition={{ duration: 0.22, ease: "easeOut" }}
              style={{
                top: mobileLayout?.top ?? 0,
                left: mobileLayout?.left ?? 0,
                width: mobileLayout?.width ?? "90vw",
                maxHeight: mobileLayout?.maxHeight ?? "80vh",
                transformOrigin: mobileLayout?.transformOrigin ?? "50% 50%",
              }}
              onClick={openFromPanel}
              className="fixed z-50 flex h-auto cursor-pointer flex-col overflow-hidden rounded-xl border border-zinc-800/50 bg-zinc-950 shadow-2xl shadow-black/80"
            >
              {/* Only the content scrolls, so the X stays pinned to the frame. */}
              <div className="flex min-h-0 flex-col overflow-y-auto overscroll-contain">
                <div className="relative w-full aspect-video shrink-0 bg-black overflow-hidden">
                  <TrailerMedia
                    isLoading={isLoading}
                    videoKey={videoKey}
                    backdrop={backdrop}
                    title={movie.title}
                    iframeClassName="h-[135%] w-[135%]"
                  />
                </div>

                <div className="flex flex-col gap-4 p-4">
                  <ExpandedInfo movie={movie} details={details} year={year} overview={overview} overviewClassName="" />
                </div>
              </div>

              <CloseButton onClose={collapse} />
            </motion.div>
          )}
        </AnimatePresence>
      </div>

      {/* Outside original title for non-hovered grid view */}
      <div className={`mt-2 space-y-0.5 transition-opacity duration-200 ${isExpanded ? 'opacity-0' : 'opacity-100'}`}>
        <h3 className="line-clamp-1 text-sm font-medium text-foreground">
          {movie.title}
        </h3>
        {year && <p className="text-xs text-muted-foreground">{year}</p>}
      </div>
    </div>
  );
});
