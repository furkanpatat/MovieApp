// TMDB CDN URLs from the paths the Catalog service returns (same as web).
const BASE = "https://image.tmdb.org/t/p";

export type PosterSize = "w185" | "w342" | "w500" | "w780";
export type BackdropSize = "w780" | "w1280" | "original";
export type ProfileSize = "w45" | "w185" | "h632";

export function posterUrl(path: string | undefined | null, size: PosterSize = "w342"): string | null {
  return path ? `${BASE}/${size}${path}` : null;
}

export function backdropUrl(path: string | undefined | null, size: BackdropSize = "w1280"): string | null {
  return path ? `${BASE}/${size}${path}` : null;
}

export function profileUrl(path: string | undefined | null, size: ProfileSize = "w185"): string | null {
  return path ? `${BASE}/${size}${path}` : null;
}
