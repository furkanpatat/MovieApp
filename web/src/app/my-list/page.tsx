"use client";

import Link from "next/link";
import { Bookmark, CloudOff, Compass, Film, type LucideIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { MovieCard } from "@/components/movies/movie-card";
import { MovieCardSkeleton } from "@/components/movies/movie-card-skeleton";
import { useAuthPrompt } from "@/store/auth-prompt-store";
import { useAuthStore } from "@/store/auth-store";
import { useLibraryStore, useUserLibrary } from "@/store/library-store";

const GRID = "grid grid-cols-2 gap-x-4 gap-y-8 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6";

export default function MyListPage() {
  const hasHydrated = useAuthStore((s) => s.hasHydrated);
  const userId = useAuthStore((s) => s.userId);
  const { list, status } = useUserLibrary();
  const openAuth = useAuthPrompt((s) => s.openAuth);

  return (
    <div className="mx-auto w-full max-w-screen-2xl flex-1 px-4 py-10 sm:px-8">
      <header className="mb-8 flex flex-wrap items-end justify-between gap-2">
        <div>
          <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">My List</h1>
          {userId && list.length > 0 && (
            <p className="mt-1 text-sm text-muted-foreground">
              {list.length} {list.length === 1 ? "title" : "titles"} · most recently added first
            </p>
          )}
        </div>
      </header>

      {!hasHydrated || (userId && (status === "idle" || status === "loading")) ? (
        <div className={GRID}>
          {Array.from({ length: 6 }).map((_, i) => (
            <MovieCardSkeleton key={i} />
          ))}
        </div>
      ) : !userId ? (
        <EmptyState
          icon={Bookmark}
          title="Sign in to build your list"
          body="Save movies and shows you want to watch, and find them all here."
          action={<Button onClick={() => openAuth()}>Sign in</Button>}
        />
      ) : status === "error" ? (
        <EmptyState
          icon={CloudOff}
          title="Couldn’t load your list"
          body="Your list is safe; we just couldn’t reach it right now."
          action={<Button onClick={() => void useLibraryStore.getState().load(userId)}>Try again</Button>}
        />
      ) : list.length === 0 ? (
        <EmptyState
          icon={Film}
          title="Your list is empty"
          body="Discover movies to add them here. Tap the bookmark on any title to save it for later."
          action={
            <div className="flex flex-wrap justify-center gap-3">
              <Button asChild>
                <Link href="/">Browse movies</Link>
              </Button>
              <Button asChild variant="secondary">
                <Link href="/discover">
                  <Compass className="size-4" />
                  Discover feed
                </Link>
              </Button>
            </div>
          }
        />
      ) : (
        <div className={GRID}>
          {list.map(({ movie }) => (
            <MovieCard key={movie.id} movie={movie} />
          ))}
        </div>
      )}
    </div>
  );
}

function EmptyState({
  icon: Icon,
  title,
  body,
  action,
}: {
  icon: LucideIcon;
  title: string;
  body: string;
  action: React.ReactNode;
}) {
  return (
    <div className="relative mx-auto flex max-w-lg flex-col items-center overflow-hidden rounded-3xl border border-white/10 bg-zinc-950/60 px-8 py-16 text-center">
      <div className="auth-aurora absolute inset-0 opacity-40 blur-2xl" aria-hidden />
      <div className="relative flex size-16 items-center justify-center rounded-2xl bg-white/5 ring-1 ring-white/10">
        <Icon className="size-8 text-primary" strokeWidth={1.75} />
      </div>
      <h2 className="relative mt-6 text-xl font-bold tracking-tight">{title}</h2>
      <p className="relative mt-2 text-sm text-muted-foreground">{body}</p>
      <div className="relative mt-8">{action}</div>
    </div>
  );
}
