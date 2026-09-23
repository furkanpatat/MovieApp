"use client";

import { Suspense, useEffect, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Loader2, Search, SearchX } from "lucide-react";

import { MovieCard } from "@/components/movies/movie-card";
import { MovieCardSkeleton } from "@/components/movies/movie-card-skeleton";
import { Button } from "@/components/ui/button";
import { normalizeQuery, useSearchMovies } from "@/hooks/queries";
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

  const search = useSearchMovies(debounced);
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
          placeholder="Search for a movie…"
          aria-label="Search movies"
          className="h-14 w-full min-w-0 bg-transparent text-lg outline-none placeholder:text-muted-foreground"
        />
        {search.isFetching && <Loader2 className="size-5 shrink-0 animate-spin text-muted-foreground" aria-label="Searching" />}
      </form>

      <div className="mt-10">
        {tooShort ? (
          <Empty icon={Search} title="Find your next movie" body="Type at least two characters to search by title." />
        ) : search.status === "pending" ? (
          <div className={GRID}>
            {Array.from({ length: 12 }).map((_, i) => (
              <MovieCardSkeleton key={i} />
            ))}
          </div>
        ) : search.status === "error" ? (
          <Empty
            icon={SearchX}
            title="Search is unavailable"
            body="We couldn't reach the movie database. Please try again."
            action={<Button onClick={() => void search.refetch()}>Try again</Button>}
          />
        ) : movies.length === 0 ? (
          <Empty icon={SearchX} title={`No results for “${debounced}”`} body="Check the spelling or try a different title." />
        ) : (
          <>
            <p className="mb-6 text-sm text-muted-foreground">
              {total.toLocaleString()} {total === 1 ? "result" : "results"} for{" "}
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
                  Load more
                </Button>
              </div>
            )}
          </>
        )}
      </div>
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
