"use client";

import { useMemo, useState } from "react";
import Image from "next/image";
import { useParams } from "next/navigation";
import { CalendarDays, MapPin, TriangleAlert, UserRound } from "lucide-react";

import { MovieRow, MovieRowSkeleton } from "@/components/movies/movie-row";
import { useT, type Locale, type MessageKey } from "@/i18n";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { usePerson } from "@/hooks/queries";
import { ApiError } from "@/lib/api-client";
import { backdropUrl, profileUrl } from "@/lib/tmdb-image";
import type { Person } from "@/types/movie";

/** Actor / crew page: photo, biography and their movies as scrolling rows.
 *  The data comes from the Catalog's Redis -> Postgres -> TMDB layers. */
export default function PersonPage() {
  const params = useParams<{ id: string }>();
  const personId = Number(params.id);
  const validId = Number.isInteger(personId) && personId > 0;
  const person = usePerson(validId ? personId : -1);
  const { t } = useT();

  if (!validId) return <ErrorState message={t("person.invalid")} />;
  if (person.status === "pending") return <PersonSkeleton />;
  if (person.status === "error") {
    const notFound = person.error instanceof ApiError && person.error.status === 404;
    return (
      <ErrorState
        message={t(notFound ? "person.notFound" : "person.loadFailed")}
        onRetry={notFound ? undefined : () => void person.refetch()}
      />
    );
  }
  return <PersonView person={person.data} />;
}

function formatDate(iso: string | undefined, locale: Locale) {
  if (!iso) return null;
  const d = new Date(`${iso}T00:00:00`);
  return Number.isNaN(d.getTime()) ? null : d.toLocaleDateString(locale, { day: "numeric", month: "long", year: "numeric" });
}

function ageAt(birthday: string, end?: string) {
  const b = new Date(`${birthday}T00:00:00`);
  const e = end ? new Date(`${end}T00:00:00`) : new Date();
  let age = e.getFullYear() - b.getFullYear();
  if (e.getMonth() < b.getMonth() || (e.getMonth() === b.getMonth() && e.getDate() < b.getDate())) age--;
  return Number.isFinite(age) && age >= 0 ? age : null;
}

function PersonView({ person: p }: { person: Person }) {
  const [bioOpen, setBioOpen] = useState(false);
  const { t, locale } = useT();
  const department = (d: string) => {
    const key = `person.departments.${d}` as MessageKey;
    const text = t(key);
    return text === key ? d : text;
  };
  const photo = profileUrl(p.profile_path, "h632");
  // Their most popular movie's backdrop sets the mood behind the header.
  const backdrop = backdropUrl(p.credits.find((c) => c.backdrop_path)?.backdrop_path, "w1280");

  const { acting, crew } = useMemo(
    () => ({
      acting: p.credits.filter((c) => c.character !== undefined || !c.job),
      crew: p.credits.filter((c) => c.job && c.character === undefined),
    }),
    [p.credits],
  );
  // Lead with what they're known for (a director's films before cameos).
  const rows =
    p.known_for_department && p.known_for_department !== "Acting"
      ? [
          { title: department(p.known_for_department), movies: crew },
          { title: t("person.acting"), movies: acting },
        ]
      : [
          { title: t("person.knownFor"), movies: acting },
          { title: t("person.behindCamera"), movies: crew },
        ];

  const born = formatDate(p.birthday, locale);
  const died = formatDate(p.deathday, locale);
  const age = p.birthday ? ageAt(p.birthday, p.deathday) : null;
  const bio = p.biography?.trim();

  return (
    <div className="flex flex-1 flex-col pb-16">
      <section className="relative overflow-hidden">
        {backdrop && (
          <Image src={backdrop} alt="" fill priority sizes="100vw" className="object-cover opacity-30" aria-hidden />
        )}
        <div className="absolute inset-0 bg-gradient-to-b from-background/60 via-background/80 to-background" aria-hidden />

        <div className="relative mx-auto flex w-full max-w-screen-2xl flex-col gap-8 px-4 pt-10 pb-6 sm:px-8 md:flex-row md:items-start md:pt-16">
          <div className="relative mx-auto aspect-2/3 w-44 shrink-0 overflow-hidden rounded-2xl bg-zinc-900 shadow-2xl shadow-black/60 ring-1 ring-white/10 sm:w-56 md:mx-0 md:w-64">
            {photo ? (
              <Image src={photo} alt={p.name} fill priority sizes="(min-width: 768px) 256px, 224px" className="object-cover" />
            ) : (
              <div className="flex size-full items-center justify-center">
                <UserRound className="size-16 text-muted-foreground" strokeWidth={1.25} />
              </div>
            )}
          </div>

          <div className="min-w-0 flex-1">
            {p.known_for_department && (
              <p className="text-sm font-semibold tracking-wide text-primary uppercase">{department(p.known_for_department)}</p>
            )}
            <h1 className="mt-1 text-4xl font-bold tracking-tight text-balance sm:text-5xl">{p.name}</h1>

            <dl className="mt-4 flex flex-wrap gap-x-6 gap-y-2 text-sm text-zinc-300">
              {born && (
                <div className="flex items-center gap-1.5">
                  <CalendarDays className="size-4 text-muted-foreground" />
                  <dt className="sr-only">{t("person.born")}</dt>
                  <dd>
                    {born}
                    {!died && age !== null && <span className="text-muted-foreground"> · {t("person.yearsOld", { n: age })}</span>}
                  </dd>
                </div>
              )}
              {died && (
                <div className="flex items-center gap-1.5">
                  <dt className="text-muted-foreground">{t("person.died")}</dt>
                  <dd>
                    {died}
                    {age !== null && <span className="text-muted-foreground"> · {t("person.aged", { n: age })}</span>}
                  </dd>
                </div>
              )}
              {p.place_of_birth && (
                <div className="flex items-center gap-1.5">
                  <MapPin className="size-4 text-muted-foreground" />
                  <dt className="sr-only">{t("person.placeOfBirth")}</dt>
                  <dd>{p.place_of_birth}</dd>
                </div>
              )}
            </dl>

            <h2 className="mt-8 text-lg font-bold tracking-tight">{t("person.biography")}</h2>
            {bio ? (
              <>
                <p
                  className={`mt-2 max-w-3xl text-sm leading-relaxed whitespace-pre-line text-zinc-300 sm:text-base ${
                    bioOpen ? "" : "line-clamp-5"
                  }`}
                >
                  {bio}
                </p>
                {bio.length > 400 && (
                  <button
                    type="button"
                    onClick={() => setBioOpen((o) => !o)}
                    className="mt-2 text-sm font-semibold text-primary underline-offset-4 hover:underline"
                  >
                    {t(bioOpen ? "person.showLess" : "person.readMore")}
                  </button>
                )}
              </>
            ) : (
              <p className="mt-2 text-sm text-muted-foreground">We don&apos;t have a biography for {p.name} yet.</p>
            )}
          </div>
        </div>
      </section>

      <div className="mx-auto w-full max-w-screen-2xl space-y-6">
        {rows.map((r) => (
          <MovieRow key={r.title} title={r.title} movies={r.movies} />
        ))}
        {p.credits.length === 0 && (
          <p className="px-4 text-sm text-muted-foreground sm:px-8">No movies to show for {p.name} yet.</p>
        )}
      </div>
    </div>
  );
}

function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-4 px-6 py-24 text-center">
      <TriangleAlert className="size-10 text-destructive" strokeWidth={1.5} />
      <p className="text-lg font-semibold">{message}</p>
      {onRetry && (
        <Button variant="secondary" onClick={onRetry}>
          Try again
        </Button>
      )}
    </div>
  );
}

function PersonSkeleton() {
  return (
    <div className="flex flex-1 flex-col pb-16">
      <div className="mx-auto flex w-full max-w-screen-2xl flex-col gap-8 px-4 pt-10 pb-6 sm:px-8 md:flex-row md:pt-16">
        <Skeleton className="mx-auto aspect-2/3 w-44 shrink-0 rounded-2xl sm:w-56 md:mx-0 md:w-64" />
        <div className="flex-1 space-y-4">
          <Skeleton className="h-4 w-24 rounded" />
          <Skeleton className="h-12 w-2/3 rounded" />
          <Skeleton className="h-5 w-1/2 rounded" />
          <Skeleton className="mt-8 h-32 w-full max-w-3xl rounded" />
        </div>
      </div>
      <MovieRowSkeleton />
    </div>
  );
}
