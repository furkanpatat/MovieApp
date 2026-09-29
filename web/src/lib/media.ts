import type { MediaType, Movie } from "@/types/movie";

export const mediaTypeOf = (m: Pick<Movie, "media_type">): MediaType => m.media_type ?? "movie";

export const isTV = (m: Pick<Movie, "media_type">) => m.media_type === "tv";

/** The title's own page: /movies/{id} or /tv/{id}. */
export function titleHref(m: Pick<Movie, "id" | "media_type">) {
  return isTV(m) ? `/tv/${m.id}` : `/movies/${m.id}`;
}

/** Unique across media types (movie 1399 and tv 1399 differ): list keys,
 *  shared-element ids. */
export function titleKey(m: Pick<Movie, "id" | "media_type">) {
  return `${mediaTypeOf(m)}-${m.id}`;
}

/** What names a title: its TMDB id and media type (movie when absent). */
export type TitleRef = Pick<Movie, "id" | "media_type">;

/** The title's API resource: /api/v1/movies/{id} or /api/v1/tv/{id}. */
export function titleApiPath(t: TitleRef) {
  return isTV(t) ? `/api/v1/tv/${t.id}` : `/api/v1/movies/${t.id}`;
}
