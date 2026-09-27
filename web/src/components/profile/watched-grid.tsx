import { MovieCardMini } from "@/components/movies/movie-card-mini";
import { titleKey } from "@/lib/media";
import type { Movie } from "@/types/movie";

/** Watched titles as a wrapping grid of mini cards (movies and series), on
 *  the profile and on the public profile page. */
export function WatchedGrid({ movies }: { movies: Movie[] }) {
  return (
    <div className="grid grid-cols-3 gap-x-3 gap-y-5 sm:grid-cols-4 md:grid-cols-5 [&>a]:w-full">
      {movies.map((m) => (
        <MovieCardMini key={titleKey(m)} movie={m} />
      ))}
    </div>
  );
}
