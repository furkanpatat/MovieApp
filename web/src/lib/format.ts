import { currentLocale, translate, type Locale } from "@/i18n";

/** "3m ago" / "2h ago" / "5d ago" style relative time, no dependency. In the
 *  UI language (components calling it use useT, so they re-render on a
 *  switch). */
export function relativeTime(iso: string, locale: Locale = currentLocale()): string {
  const diffMs = Date.now() - new Date(iso).getTime();
  const s = Math.max(0, Math.round(diffMs / 1000));
  if (s < 5) return translate(locale, "common.justNow");
  if (s < 60) return translate(locale, "common.secondsAgo", { n: s });
  const m = Math.round(s / 60);
  if (m < 60) return translate(locale, "common.minutesAgo", { n: m });
  const h = Math.round(m / 60);
  if (h < 24) return translate(locale, "common.hoursAgo", { n: h });
  const d = Math.round(h / 24);
  if (d < 30) return translate(locale, "common.daysAgo", { n: d });
  return new Date(iso).toLocaleDateString(locale);
}

/**
 * The Interaction service only knows a commenter's user_id (a UUID) — it has
 * no cross-service lookup to Auth for a display name. "You" for the current
 * user is the one identity we know for certain; everyone else is shown as a
 * short, stable, colored id badge rather than the raw UUID.
 */
export function displayName(userId: string, currentUserId: string | null, locale: Locale = currentLocale()): string {
  if (currentUserId && userId === currentUserId) return translate(locale, "common.you");
  return translate(locale, "common.userShort", { id: userId.slice(0, 8) });
}

/** Compact counts: 1.2K (EN), 1,2 B (TR). */
export function compactNumber(n: number, locale: Locale = currentLocale()): string {
  return new Intl.NumberFormat(locale, { notation: "compact", maximumFractionDigits: 1 }).format(n);
}

/** Two-letter avatar initials matching displayName: "Y" for the current
 *  user, or the first two characters of their id otherwise — derived from
 *  the id itself, not from the "User " label, so different users are
 *  visually distinguishable (not every stranger reading "US"). */
export function initials(userId: string, currentUserId: string | null, locale: Locale = currentLocale()): string {
  if (currentUserId && userId === currentUserId) return translate(locale, "common.you").charAt(0);
  return userId.slice(0, 2).toUpperCase();
}

/** Deterministic color for an id, so the same user always gets the same
 *  avatar color without a real profile to draw one from. */
export function idColor(id: string): string {
  let hash = 0;
  for (let i = 0; i < id.length; i++) hash = (hash * 31 + id.charCodeAt(i)) >>> 0;
  const hue = hash % 360;
  return `hsl(${hue} 55% 45%)`;
}
