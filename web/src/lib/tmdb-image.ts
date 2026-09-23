const TMDB_IMAGE_BASE = "https://image.tmdb.org/t/p";

export type PosterSize = "w185" | "w342" | "w500" | "w780";
export type BackdropSize = "w780" | "w1280" | "original";
export type ProfileSize = "w45" | "w185" | "h632";

/** Builds a TMDB CDN URL from the path the Catalog service returns, or null
 *  when there's no image (some titles genuinely have none). */
export function posterUrl(path: string | undefined, size: PosterSize = "w500"): string | null {
  return path ? `${TMDB_IMAGE_BASE}/${size}${path}` : null;
}

export function backdropUrl(path: string | undefined, size: BackdropSize = "w1280"): string | null {
  return path ? `${TMDB_IMAGE_BASE}/${size}${path}` : null;
}

export function profileUrl(path: string | undefined | null, size: ProfileSize = "w185"): string | null {
  return path ? `${TMDB_IMAGE_BASE}/${size}${path}` : null;
}
