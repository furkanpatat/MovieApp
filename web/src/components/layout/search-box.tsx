"use client";

import { useEffect, useId, useRef, useState } from "react";
import Image from "next/image";
import { usePathname, useRouter } from "next/navigation";
import { Clapperboard, Loader2, Search, X } from "lucide-react";

import { RatingBadge } from "@/components/movies/rating-badge";
import { normalizeQuery, useSearchTitles } from "@/hooks/queries";
import { useT } from "@/i18n";
import { titleHref } from "@/lib/media";
import { useMediaMode } from "@/store/media-mode-store";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { posterUrl } from "@/lib/tmdb-image";
import type { Movie } from "@/types/movie";

const QUICK_RESULTS = 6;
const MIN_CHARS = 2;

export function searchHref(q: string) {
  return `/search?q=${encodeURIComponent(normalizeQuery(q))}`;
}

/**
 * Header search: typing shows a quick-results dropdown (debounced, so the
 * API sees one request per pause, not per keystroke). Arrow keys move
 * through the results, Enter opens the highlighted movie or, with none
 * highlighted, the full /search page; Escape closes.
 */
export function SearchBox() {
  const router = useRouter();
  const { t } = useT();
  const pathname = usePathname();
  const listId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const [value, setValue] = useState("");
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);

  const debounced = useDebouncedValue(value, 300);
  // Searches the app's current mode: movies, or series.
  const { mode, ready: modeReady } = useMediaMode();
  const search = useSearchTitles(debounced, mode, modeReady);
  const results: Movie[] = (search.data?.pages[0]?.results ?? []).slice(0, QUICK_RESULTS);
  const ready = normalizeQuery(value).length >= MIN_CHARS;
  // Waiting on the debounce or the request: show a spinner, not stale "no results".
  const loading = ready && (value !== debounced || search.isFetching);
  const showPanel = open && ready;

  // Navigating anywhere closes the dropdown.
  const [shownOn, setShownOn] = useState(pathname);
  if (pathname !== shownOn) {
    setShownOn(pathname);
    setOpen(false);
    setActive(-1);
  }

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [open]);

  const goTo = (href: string) => {
    setOpen(false);
    inputRef.current?.blur();
    router.push(href);
  };
  const openMovie = (m: Movie) => {
    setValue("");
    goTo(titleHref(m));
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Escape") {
      setOpen(false);
      inputRef.current?.blur();
    } else if (e.key === "ArrowDown" && results.length > 0) {
      e.preventDefault();
      setOpen(true);
      setActive((i) => (i + 1) % results.length);
    } else if (e.key === "ArrowUp" && results.length > 0) {
      e.preventDefault();
      setActive((i) => (i <= 0 ? results.length - 1 : i - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (active >= 0 && results[active]) openMovie(results[active]);
      else if (ready) goTo(searchHref(value));
    }
  };

  return (
    <div ref={rootRef} className="relative w-full max-w-sm">
      <div className="flex items-center gap-2 rounded-full border border-white/10 bg-white/5 px-4 text-sm transition-colors focus-within:border-primary/60 focus-within:bg-white/10 hover:border-white/20">
        <Search className="size-4 shrink-0 text-muted-foreground" aria-hidden />
        <input
          ref={inputRef}
          type="search"
          value={value}
          onChange={(e) => {
            setValue(e.target.value);
            setOpen(true);
            setActive(-1);
          }}
          onFocus={() => setOpen(true)}
          onKeyDown={onKeyDown}
          placeholder={t(mode === "tv" ? "search.series" : "search.movies")}
          aria-label={t(mode === "tv" ? "search.seriesLabel" : "search.moviesLabel")}
          role="combobox"
          aria-expanded={showPanel}
          aria-controls={listId}
          aria-autocomplete="list"
          aria-activedescendant={active >= 0 ? `${listId}-${active}` : undefined}
          className="h-9 w-full min-w-0 bg-transparent text-foreground outline-none placeholder:text-muted-foreground [&::-webkit-search-cancel-button]:hidden"
        />
        {loading ? (
          <Loader2 className="size-4 shrink-0 animate-spin text-muted-foreground" aria-label={t("search.searching")} />
        ) : (
          value && (
            <button
              type="button"
              aria-label={t("search.clear")}
              onClick={() => {
                setValue("");
                inputRef.current?.focus();
              }}
              className="rounded-full p-0.5 text-muted-foreground hover:text-foreground"
            >
              <X className="size-4" />
            </button>
          )
        )}
      </div>

      {showPanel && (
        <div className="absolute inset-x-0 top-full z-50 mt-2 overflow-hidden rounded-2xl border border-white/10 bg-zinc-950/95 shadow-2xl shadow-black/60 backdrop-blur-xl">
          <ul id={listId} role="listbox" aria-label={t("search.quickResults")} className="max-h-[60vh] overflow-y-auto py-1">
            {results.map((m, i) => {
              const poster = posterUrl(m.poster_path, "w185");
              return (
                <li
                  key={m.id}
                  id={`${listId}-${i}`}
                  role="option"
                  aria-selected={i === active}
                  onPointerDown={(e) => e.preventDefault()} // keep focus in the input
                  onPointerEnter={() => setActive(i)}
                  onClick={() => openMovie(m)}
                  className={`flex cursor-pointer items-center gap-3 px-3 py-2 ${i === active ? "bg-white/10" : ""}`}
                >
                  <div className="relative h-14 w-10 shrink-0 overflow-hidden rounded bg-zinc-900">
                    {poster ? (
                      <Image src={poster} alt="" fill sizes="40px" className="object-cover" />
                    ) : (
                      <Clapperboard className="m-auto mt-4 size-5 text-muted-foreground" />
                    )}
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-foreground">{m.title}</p>
                    <div className="mt-0.5 flex items-center gap-2 text-xs text-muted-foreground">
                      {m.release_date && <span>{m.release_date.slice(0, 4)}</span>}
                      <RatingBadge movie={m} size="sm" />
                    </div>
                  </div>
                </li>
              );
            })}
          </ul>

          {!loading && results.length === 0 && (
            <p className="px-4 py-6 text-center text-sm text-muted-foreground">
              {search.isError
                ? t("search.unavailable")
                : t(mode === "tv" ? "search.noSeriesMatch" : "search.noMoviesMatch", { q: normalizeQuery(value) })}
            </p>
          )}

          <button
            type="button"
            onPointerDown={(e) => e.preventDefault()}
            onClick={() => goTo(searchHref(value))}
            className="flex w-full items-center gap-2 border-t border-white/10 px-4 py-3 text-left text-sm font-medium text-primary hover:bg-white/5"
          >
            <Search className="size-4" />
            {t("search.seeAll", { q: normalizeQuery(value) })}
          </button>
        </div>
      )}
    </div>
  );
}
