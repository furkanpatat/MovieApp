import { useWindowDimensions } from "react-native";

/** A tablet: the shorter side is at least this many dp (Android's sw600dp). */
export const TABLET_MIN_SIDE = 600;
/** Text, forms and details read best at this width; wider screens center it. */
export const READABLE_WIDTH = 760;

/**
 * One source of sizes for every screen, from the window (so it follows
 * rotation and split screen): phones keep the tight layout, tablets get
 * bigger posters, more grid columns and a centered reading column.
 */
export function useLayout() {
  const { width, height } = useWindowDimensions();
  const tablet = Math.min(width, height) >= TABLET_MIN_SIDE;
  const landscape = width > height;
  const gutter = tablet ? 24 : 16;
  // ~3.4 posters across a phone, ~7.5 across a tablet; 110-180dp wide.
  const poster = Math.round(Math.min(180, Math.max(110, width / (tablet ? 7.5 : 3.4))));
  const columns = Math.max(3, Math.floor((width - gutter * 2) / (tablet ? 150 : 120)));
  // The hero: tall on a phone held upright, a banner on wide screens.
  const hero = Math.round(landscape || tablet ? Math.min(height * 0.72, width * 0.6) : Math.min(width * 1.25, height * 0.8));
  return { width, height, tablet, landscape, gutter, poster, columns, hero, readable: Math.min(width, READABLE_WIDTH) };
}
