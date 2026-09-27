"use client";

import { Suspense, useEffect, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Loader2, Search, SearchX } from "lucide-react";

import { MovieCard } from "@/components/movies/movie-card";
import { MovieCardSkeleton } from "@/components/movies/movie-card-skeleton";
import { Button } from "@/components/ui/button";
import { normalizeQuery, useSearchTitles } from "@/hooks/queries";
import { plural, useT, type MessageKey } from "@/i18n";
import { cn } from "@/lib/utils";
import { useMediaMode, useMediaModeStore } from "@/store/media-mode-store";
import type { MediaType } from "@/types/movie";
import { useDebouncedValue } from "@/hooks/use-debounced-value";

const GRID = "grid grid-cols-2 gap-x-4 gap-y-8 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6";

// useSearchParams makes this client-rendered; the Suspense boundary lets the
// shell prerender (see Next's useSearchParams docs).
export default function SearchPage() {
  return (
    <Suspense fallback={<SearchShell query="" />}>
      <SearchResults />
    </Suspense>
  );
}

// The last ?q= this page wrote itself (while typing). Only a query from
// elsewhere, e.g. the header search, should replace what's in the input.
let ownQuery: string | null = null;

function SearchResults() {
  const params = useSearchParams();
  return <SearchShell query={params.get("q") ?? ""} />;
}

function SearchShell({ query }: { query: string }) {
  const router = useRouter();
  const pathname = usePathname();
  const [value, setValue] = useState(query);
  const [seenQuery, setSeenQuery] = useState(query);
  if (query !== seenQuery) {
    setSeenQuery(query);
    if (query !== ownQuery && normalizeQuery(query) !== normalizeQuery(value)) setValue(query);
  }
  const debounced = normalizeQuery(useDebouncedValue(value, 350));

  // Keep ?q= in step with what's typed (replace: no history entry per pause).
  useEffect(() => {
    if (debounced === normalizeQuery(query)) return;
    ownQuery = debounced;
    router.replace(debounced ? `${pathname}?q=${encodeURIComponent(debounced)}` : pathname, { scroll: false });
  }, [debounced, query, pathname, router]);

  // Movies or series: the app's mode, which the toggle below also switches.
  const { mode, ready } = useMediaMode();
  const search = useSearchTitles(debounced, mode, ready);
  const { t, locale } = useT();
  const tv = mode === "tv";
  const movies = search.data?.pages.flatMap((p) => p.results) ?? [];
  const total = search.data?.pages[0]?.total_results ?? 0;
  const tooShort = debounced.length < 2;

  return (
    <div className="mx-auto w-full max-w-screen-2xl flex-1 px-4 py-10 sm:px-8">
      <form
        role="search"
        onSubmit={(e) => e.preventDefault()}
        className="mx-auto flex max-w-2xl items-center gap-3 rounded-full border border-white/10 bg-white/5 px-5 transition-colors focus-within:border-primary/60 focus-within:bg-white/10"
      >
        <Search className="size-5 shrink-0 text-muted-foreground" aria-hidden />
        <input
          type="search"
          autoFocus
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder={t(tv ? "search.forSeries" : "search.forMovie")}
          aria-label={t(tv ? "search.seriesLabel" : "search.moviesLabel")}
          className="h-14 w-full min-w-0 bg-transparent text-lg outline-none placeholder:text-muted-foreground"
        />
        {search.isFetching && <Loader2 className="size-5 shrink-0 animate-spin text-muted-foreground" aria-label={t("search.searching")} />}
      </form>

      <ModeToggle mode={mode} />

      <div className="mt-8">
        {tooShort ? (
          <Empty icon={Search} title={t(tv ? "search.findSeries" : "search.findMovie")} body={t("search.typeMore")} />
        ) : search.status === "pending" ? (
          <div className={GRID}>
            {Array.from({ length: 12 }).map((_, i) => (
              <MovieCardSkeleton key={i} />
            ))}
          </div>
        ) : search.status === "error" ? (
          <Empty
            icon={SearchX}
            title={t("search.unavailableTitle")}
            body={t("search.unavailableBody")}
            action={<Button onClick={() => void search.refetch()}>{t("common.tryAgain")}</Button>}
          />
        ) : movies.length === 0 ? (
          <Empty icon={SearchX} title={t("search.noResults", { q: debounced })} body={t("search.checkSpelling")} />
        ) : (
          <>
            <p className="mb-6 text-sm text-muted-foreground">
              {t(plural(total, "search.resultsOne", "search.resultsOther"), { n: total.toLocaleString(locale) })}{" "}
              <span className="font-semibold text-foreground">“{debounced}”</span>
            </p>
            <div className={GRID}>
              {movies.map((m) => (
                <MovieCard key={m.id} movie={m} />
              ))}
            </div>
            {search.hasNextPage && (
              <div className="mt-10 flex justify-center">
                <Button variant="secondary" onClick={() => void search.fetchNextPage()} disabled={search.isFetchingNextPage}>
                  {search.isFetchingNextPage && <Loader2 className="size-4 animate-spin" />}
                  {t("search.loadMore")}
                </Button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

const MODES = [
  { value: "movie", label: "mode.movies" },
  { value: "tv", label: "mode.series" },
] as const satisfies { value: MediaType; label: MessageKey }[];

/** Movies | Series, a segmented control over the app-wide mode (the logo). */
function ModeToggle({ mode }: { mode: MediaType }) {
  const setMode = useMediaModeStore((s) => s.setMode);
  const { t } = useT();
  return (
    <div role="radiogroup" aria-label={t("search.searchIn")} className="mx-auto mt-4 flex w-fit rounded-full border border-white/10 bg-white/5 p-1">
      {MODES.map((m) => (
        <button
          key={m.value}
          type="button"
          role="radio"
          aria-checked={mode === m.value}
          onClick={() => setMode(m.value)}
          className={cn(
            "rounded-full px-4 py-1.5 text-sm transition-colors outline-none focus-visible:ring-2 focus-visible:ring-primary",
            mode === m.value ? "bg-white font-semibold text-black" : "font-medium text-white/70 hover:text-white",
          )}
        >
          {t(m.label)}
        </button>
      ))}
    </div>
  );
}

function Empty({
  icon: Icon,
  title,
  body,
  action,
}: {
  icon: typeof Search;
  title: string;
  body: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="mx-auto flex max-w-md flex-col items-center py-16 text-center">
      <div className="flex size-14 items-center justify-center rounded-2xl bg-white/5 ring-1 ring-white/10">
        <Icon className="size-7 text-primary" strokeWidth={1.75} />
      </div>
      <h2 className="mt-5 text-xl font-bold tracking-tight">{title}</h2>
      <p className="mt-2 text-sm text-muted-foreground">{body}</p>
      {action && <div className="mt-6">{action}</div>}
    </div>
  );
}
