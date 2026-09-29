import type { Locale } from "@/i18n";
import type { MediaType } from "@/types/movie";

/** TMDB genres for the Discover filter, as on the web (web/src/components/
 *  discover/genre-picker.tsx); id 0 is the unfiltered "For You" feed. TV has
 *  its own, broader ids. Names: [English, Turkish]. */
type Genre = { id: number; name: [string, string] };

const MOVIE: Genre[] = [
  { id: 0, name: ["For You", "Senin İçin"] },
  { id: 28, name: ["Action", "Aksiyon"] },
  { id: 878, name: ["Sci-Fi", "Bilim Kurgu"] },
  { id: 18, name: ["Drama", "Dram"] },
  { id: 35, name: ["Comedy", "Komedi"] },
  { id: 53, name: ["Thriller", "Gerilim"] },
  { id: 27, name: ["Horror", "Korku"] },
  { id: 12, name: ["Adventure", "Macera"] },
  { id: 16, name: ["Animation", "Animasyon"] },
  { id: 80, name: ["Crime", "Suç"] },
  { id: 14, name: ["Fantasy", "Fantastik"] },
  { id: 10749, name: ["Romance", "Romantik"] },
  { id: 9648, name: ["Mystery", "Gizem"] },
  { id: 10751, name: ["Family", "Aile"] },
  { id: 36, name: ["History", "Tarih"] },
  { id: 10752, name: ["War", "Savaş"] },
  { id: 37, name: ["Western", "Western"] },
  { id: 99, name: ["Documentary", "Belgesel"] },
];

const TV: Genre[] = [
  { id: 0, name: ["For You", "Senin İçin"] },
  { id: 18, name: ["Drama", "Dram"] },
  { id: 10765, name: ["Sci-Fi & Fantasy", "Bilim Kurgu ve Fantastik"] },
  { id: 10759, name: ["Action & Adventure", "Aksiyon ve Macera"] },
  { id: 35, name: ["Comedy", "Komedi"] },
  { id: 80, name: ["Crime", "Suç"] },
  { id: 9648, name: ["Mystery", "Gizem"] },
  { id: 16, name: ["Animation", "Animasyon"] },
  { id: 10768, name: ["War & Politics", "Savaş ve Politika"] },
  { id: 10751, name: ["Family", "Aile"] },
  { id: 10762, name: ["Kids", "Çocuk"] },
  { id: 99, name: ["Documentary", "Belgesel"] },
  { id: 37, name: ["Western", "Western"] },
];

export const GENRES: Record<MediaType, Genre[]> = { movie: MOVIE, tv: TV };

export function genreLabel(mode: MediaType, id: number, locale: Locale): string {
  const g = GENRES[mode].find((x) => x.id === id) ?? GENRES[mode][0];
  return g.name[locale === "tr" ? 1 : 0];
}
