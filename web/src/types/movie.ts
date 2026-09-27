/**
 * Wire types for the Catalog service, reached via the Gateway at
 * /api/v1/movies/*. Field names mirror the JSON exactly (see
 * services/catalog/internal/domain/movie.go) rather than being remapped to
 * camelCase, so there's one obviously-correct source of truth instead of a
 * mapping layer that can drift from the backend.
 */
export interface Genre {
  id: number;
  name: string;
}

/** "movie" or "tv". TMDB numbers them separately (movie 1399 and tv 1399
 *  are different titles), so a title is identified by both. */
export type MediaType = "movie" | "tv";

/** A title: a movie, or a TV series (media_type "tv") in the same shape.
 *  For a series, title is its name, release_date its first air date and
 *  runtime the episode length. */
export interface Movie {
  id: number;
  /** Absent on older cached responses: treat as "movie". */
  media_type?: MediaType;
  title: string;
  overview: string;
  poster_path?: string;
  backdrop_path?: string;
  release_date?: string;
  vote_average: number;
  vote_count: number;
  // Only populated by the movie-details endpoint.
  tagline?: string;
  runtime?: number;
  genres?: Genre[];
  trailer_key?: string;
  cast_json?: string;
  /** IMDb's own rating (via OMDb), when known. vote_average is TMDB's score. */
  imdb_id?: string;
  imdb_rating?: number;
  imdb_votes?: number;
  /** From OMDb, stored with the IMDb rating (movie details only). */
  rated?: string;
  rotten_tomatoes?: string;
  metascore?: number;
  awards?: string;
  director?: string;
  writer?: string;
  box_office?: string;
  country?: string;
  language?: string;
  /** TV series only (details). */
  number_of_seasons?: number;
  number_of_episodes?: number;
  /** "Returning Series", "Ended", "Canceled"... */
  status?: string;
  last_air_date?: string;
  networks?: string[];
  creators?: string[];
}

/** One movie in a person's filmography: a movie plus their role on it. */
export interface Credit extends Movie {
  character?: string;
  job?: string;
}

/** GET /api/v1/people/{id} */
export interface Person {
  id: number;
  name: string;
  biography?: string;
  profile_path?: string;
  birthday?: string;
  deathday?: string;
  place_of_birth?: string;
  known_for_department?: string;
  credits: Credit[];
}

export interface MoviePage {
  page: number;
  total_pages: number;
  total_results: number;
  results: Movie[];
}

/** A comment as returned by the Interaction service's read model. */
export interface Comment {
  id: string;
  user_id: string;
  text: string;
  created_at: string;
}

/** GET /api/v1/movies/{id}/interactions — served from the Redis read model,
 *  eventually consistent with the CQRS write path (rate/comment return 202
 *  before this reflects them; see the optimistic updates in queries.ts). */
export interface Interactions {
  movie_id: number;
  average_rating: number;
  total_votes: number;
  recent_comments: Comment[];
}
