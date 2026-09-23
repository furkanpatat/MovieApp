"use client";

import { Check } from "lucide-react";

import { StarRatingDisplay, StarRatingInput } from "@/components/movies/star-rating";
import { useRateMovie } from "@/hooks/queries";
import { useMyRating, useRequireAuth } from "@/hooks/use-library";
import type { Interactions, Movie } from "@/types/movie";

/** Community average plus the user's own stars. The stars are shown to
 *  everyone; clicking one while signed out opens sign-in instead. */
export function RatingSection({ movie, interactions }: { movie: Movie; interactions: Interactions }) {
  const rate = useRateMovie(movie.id, movie);
  const myRating = useMyRating(movie.id);
  const requireAuth = useRequireAuth();
  // Show the score being sent right away, then the stored one.
  const shown = rate.isPending ? rate.variables : myRating;

  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
      <StarRatingDisplay value={interactions.average_rating} votes={interactions.total_votes} />

      <div className="h-6 w-px bg-border" />

      <div className="flex items-center gap-2">
        <StarRatingInput
          value={shown}
          disabled={rate.isPending}
          onSelect={(score) => requireAuth(() => rate.mutate(score))}
        />
        {myRating !== undefined && !rate.isPending ? (
          <span className="flex items-center gap-1 text-sm text-muted-foreground">
            <Check className="size-3.5 text-primary" />
            You rated {myRating}/10
          </span>
        ) : (
          !rate.isPending && <span className="text-sm text-muted-foreground">Rate it</span>
        )}
        {rate.isError && <span className="text-sm text-red-300">Couldn&apos;t save your rating.</span>}
      </div>
    </div>
  );
}
