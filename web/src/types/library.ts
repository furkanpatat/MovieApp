/**
 * Wire types for the user library, served by the Catalog service through the
 * Gateway (session required). The user is always the signed-in one; nothing
 * here names a user id. See services/catalog/internal/domain/library.go.
 */
import type { Movie } from "@/types/movie";

/** The title part of a library row (a movie or a series, per media_type):
 *  the summary fields a card renders. */
export type LibraryMovie = Pick<
  Movie,
  | "id"
  | "media_type"
  | "title"
  | "overview"
  | "poster_path"
  | "backdrop_path"
  | "release_date"
  | "vote_average"
  | "vote_count"
  | "imdb_rating"
>;

/** GET /api/v1/watchlist items, POST /api/v1/watchlist response. */
export interface WatchlistItem {
  movie: LibraryMovie;
  added_at: string;
}

/** GET /api/v1/ratings items, PUT /api/v1/ratings response. */
export interface UserRating {
  movie: LibraryMovie;
  rating: number;
  created_at: string;
  updated_at: string;
}

export interface ListResponse<T> {
  items: T[];
}
