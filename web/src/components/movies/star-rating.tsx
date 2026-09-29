"use client";

import { plural, useT } from "@/i18n";
import { useState } from "react";
import { Star } from "lucide-react";

const STAR_COUNT = 5;

/** Fill percentage (0-100) for star index `i` (0-based) given a 0-10 value. */
function fillFor(i: number, value: number): number {
  const starValue = value - i * 2; // this star's share of the 0-2 range
  return Math.max(0, Math.min(100, starValue * 50));
}

function StarShape({ fillPct, className }: { fillPct: number; className: string }) {
  return (
    <span className={`relative inline-block ${className}`}>
      <Star className="size-full text-muted-foreground/40" strokeWidth={1.5} />
      <span className="absolute inset-0 overflow-hidden" style={{ width: `${fillPct}%` }}>
        <Star className="size-full fill-primary text-primary" strokeWidth={1.5} />
      </span>
    </span>
  );
}

/** Read-only aggregate rating, e.g. in the movie header. */
export function StarRatingDisplay({
  value,
  votes,
  starClassName = "size-5",
}: {
  value: number;
  votes: number;
  starClassName?: string;
}) {
  const { t, locale } = useT();
  return (
    <div className="flex items-center gap-2">
      <div className="flex items-center gap-0.5">
        {Array.from({ length: STAR_COUNT }).map((_, i) => (
          <StarShape key={i} fillPct={fillFor(i, value)} className={starClassName} />
        ))}
      </div>
      <span className="text-sm font-semibold text-foreground">{value > 0 ? value.toFixed(1) : "—"}</span>
      <span className="text-sm text-muted-foreground">
        ({t(plural(votes, "rating.votesOne", "rating.votesOther"), { n: votes.toLocaleString(locale) })})
      </span>
    </div>
  );
}

/**
 * Interactive 1-10 picker rendered as 5 stars, each half independently
 * clickable — the same half-star convention Letterboxd uses, and a natural
 * fit here since the backend's score is already an integer 1-10 (two steps
 * per star, no rounding needed in either direction).
 */
/** Controlled: `value` is the score to show (e.g. the user's own rating), so a
 *  click that doesn't result in a rating (signed out) leaves nothing lit. */
export function StarRatingInput({
  value,
  onSelect,
  disabled,
}: {
  value?: number;
  onSelect: (score: number) => void;
  disabled?: boolean;
}) {
  const { t } = useT();
  const [hover, setHover] = useState<number | null>(null);
  const active = hover ?? value ?? 0;

  return (
    <div
      className={`flex items-center gap-0.5 ${disabled ? "pointer-events-none opacity-50" : ""}`}
      onMouseLeave={() => setHover(null)}
    >
      {Array.from({ length: STAR_COUNT }).map((_, i) => {
        const leftValue = i * 2 + 1;
        const rightValue = i * 2 + 2;
        return (
          <span key={i} className="relative inline-block size-7 cursor-pointer">
            <StarShape fillPct={fillFor(i, active)} className="size-full" />
            <button
              type="button"
              aria-label={t("rating.rateOutOf", { n: leftValue })}
              className="absolute inset-y-0 left-0 w-1/2"
              onMouseEnter={() => setHover(leftValue)}
              onClick={() => {
                onSelect(leftValue);
              }}
            />
            <button
              type="button"
              aria-label={t("rating.rateOutOf", { n: rightValue })}
              className="absolute inset-y-0 right-0 w-1/2"
              onMouseEnter={() => setHover(rightValue)}
              onClick={() => {
                onSelect(rightValue);
              }}
            />
          </span>
        );
      })}
    </div>
  );
}
