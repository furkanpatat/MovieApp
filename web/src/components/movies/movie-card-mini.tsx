import Image from "next/image";
import Link from "next/link";
import { Clapperboard } from "lucide-react";

import { RatingBadge } from "@/components/movies/rating-badge";
import { titleHref } from "@/lib/media";
import { posterUrl } from "@/lib/tmdb-image";
import type { Movie } from "@/types/movie";

/**
 * A compact MovieCard for tight spaces (chat bubbles): poster, title, year
 * and rating, linking to the movie. It deliberately skips MovieCard's hover
 * preview: that preview is position:fixed, which breaks inside containers
 * with backdrop-filter (like the assistant panel) and has no room there.
 */
export function MovieCardMini({ movie, onNavigate }: { movie: Movie; onNavigate?: () => void }) {
  const poster = posterUrl(movie.poster_path, "w185");
  const year = movie.release_date?.slice(0, 4);
  return (
    <Link
      href={titleHref(movie)}
      onClick={onNavigate}
      className="group block w-28 shrink-0 snap-start rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-primary/60"
    >
      <div className="relative aspect-2/3 overflow-hidden rounded-lg bg-zinc-900 ring-1 ring-white/10 transition group-hover:ring-primary/60">
        {poster ? (
          <Image
            src={poster}
            alt={movie.title}
            fill
            sizes="112px"
            className="object-cover transition-transform duration-300 group-hover:scale-105"
          />
        ) : (
          <div className="flex size-full items-center justify-center">
            <Clapperboard className="size-6 text-muted-foreground" strokeWidth={1.5} />
          </div>
        )}
      </div>
      <p className="mt-1.5 line-clamp-1 text-xs font-semibold text-foreground group-hover:text-primary">{movie.title}</p>
      <div className="mt-0.5 flex items-center gap-1.5 text-[11px] text-muted-foreground">
        {year && <span>{year}</span>}
        <RatingBadge movie={movie} size="sm" />
      </div>
    </Link>
  );
}
