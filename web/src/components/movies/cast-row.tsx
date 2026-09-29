"use client";

import { memo, useMemo } from "react";
import Image from "next/image";
import Link from "next/link";

import { useT } from "@/i18n";
import { profileUrl } from "@/lib/tmdb-image";

interface CastMember {
  id: number;
  name: string;
  character?: string;
  profile_path?: string | null;
  order?: number;
}

const MAX_CAST = 20;

/** cast_json is TMDB's credits.cast, stored verbatim by the catalog. Parse
 *  defensively: it can be absent, empty, or (for old cache rows) malformed. */
function parseCast(castJson?: string): CastMember[] {
  if (!castJson) return [];
  try {
    const raw: unknown = JSON.parse(castJson);
    if (!Array.isArray(raw)) return [];
    return raw
      .filter((c): c is CastMember => typeof c?.id === "number" && typeof c?.name === "string")
      .sort((a, b) => (a.order ?? 999) - (b.order ?? 999))
      .slice(0, MAX_CAST);
  } catch {
    return [];
  }
}

function initials(name: string) {
  return name
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase();
}

/** Top-billed cast as a horizontal rail, styled like the home page rows. */
export const CastRow = memo(function CastRow({ castJson }: { castJson?: string }) {
  const cast = useMemo(() => parseCast(castJson), [castJson]);
  const { t } = useT();
  if (cast.length === 0) return null;

  return (
    <section aria-label={t("detail.cast")}>
      <h2 className="mb-1 text-xl font-bold tracking-tight">{t("detail.topCast")}</h2>
      <div className="hide-scrollbar -mx-4 flex snap-x snap-mandatory scroll-px-4 gap-4 overflow-x-auto px-4 py-3 sm:-mx-8 sm:scroll-px-8 sm:px-8">
        {cast.map((c) => {
          const photo = profileUrl(c.profile_path);
          return (
            <Link
              key={`${c.id}-${c.character}`}
              href={`/person/${c.id}`}
              className="group w-28 shrink-0 snap-start rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-primary/60 sm:w-32"
            >
              <div className="relative aspect-[2/3] overflow-hidden rounded-xl bg-zinc-900 ring-1 ring-white/10 transition group-hover:ring-primary/60">
                {photo ? (
                  <Image src={photo} alt={c.name} fill sizes="128px" className="object-cover transition-transform duration-300 group-hover:scale-105" />
                ) : (
                  <div className="flex size-full items-center justify-center text-2xl font-semibold text-zinc-500">
                    {initials(c.name)}
                  </div>
                )}
              </div>
              <p className="mt-2 line-clamp-1 text-sm font-semibold transition-colors group-hover:text-primary">{c.name}</p>
              {c.character && <p className="line-clamp-1 text-xs text-muted-foreground">{c.character}</p>}
            </Link>
          );
        })}
      </div>
    </section>
  );
});
