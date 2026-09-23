import { cn } from "@/lib/utils";
import type { Movie } from "@/types/movie";

type Size = "sm" | "md" | "lg";

const LOGO: Record<Size, string> = {
  sm: "h-4 px-1 text-[10px] rounded-[3px]",
  md: "h-5 px-1.5 text-xs rounded",
  lg: "h-6 px-1.5 text-sm rounded",
};
const SCORE: Record<Size, string> = { sm: "text-xs", md: "text-sm", lg: "text-base" };

/** IMDb's wordmark: black heavy type on IMDb yellow. */
export function IMDbLogo({ size = "md", className }: { size?: Size; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn(
        "inline-flex items-center bg-[#F5C518] font-black leading-none tracking-tight text-black",
        LOGO[size],
        className,
      )}
    >
      IMDb
    </span>
  );
}

function TMDBLogo({ size = "md" }: { size?: Size }) {
  return (
    <span
      aria-hidden
      className={cn(
        "inline-flex items-center bg-gradient-to-r from-[#90cea1] to-[#01b4e4] font-black leading-none tracking-tight text-[#0d253f]",
        LOGO[size],
      )}
    >
      TMDB
    </span>
  );
}

function compactVotes(n: number) {
  return new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(n);
}

/**
 * The movie's rating with its source made explicit. IMDb's own rating when
 * the catalog has it (fetched from OMDb); otherwise TMDB's community score,
 * labelled as such, never passed off as IMDb's. Nothing when neither exists.
 */
export function RatingBadge({
  movie,
  size = "md",
  showVotes = false,
  className,
}: {
  movie: Pick<Movie, "imdb_rating" | "imdb_votes" | "vote_average">;
  size?: Size;
  showVotes?: boolean;
  className?: string;
}) {
  const imdb = movie.imdb_rating ?? 0;
  const source = imdb > 0 ? "IMDb" : movie.vote_average > 0 ? "TMDB" : null;
  if (!source) return null;
  const score = source === "IMDb" ? imdb : movie.vote_average;

  return (
    <span
      className={cn("inline-flex items-center gap-1.5", className)}
      aria-label={`${source} rating ${score.toFixed(1)} out of 10`}
      title={source === "IMDb" ? "IMDb rating" : "TMDB user score (no IMDb rating yet)"}
    >
      {source === "IMDb" ? <IMDbLogo size={size} /> : <TMDBLogo size={size} />}
      <span className={cn("font-bold tabular-nums text-foreground", SCORE[size])}>
        {score.toFixed(1)}
        <span className="font-medium text-muted-foreground">/10</span>
      </span>
      {showVotes && source === "IMDb" && movie.imdb_votes ? (
        <span className={cn("text-muted-foreground", SCORE.sm)}>({compactVotes(movie.imdb_votes)})</span>
      ) : null}
    </span>
  );
}
