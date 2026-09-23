"use client";

import { useEffect, type ReactNode } from "react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Bookmark, KeyRound, LogOut, Mail, Star, UserRound } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useLogout } from "@/hooks/use-auth";
import { relativeTime } from "@/lib/format";
import { posterUrl } from "@/lib/tmdb-image";
import { useAuthStore } from "@/store/auth-store";
import { useLibraryStore, useUserLibrary } from "@/store/library-store";

const GLASS = "rounded-2xl border border-white/10 bg-zinc-950/60 shadow-2xl shadow-black/30 backdrop-blur-xl";

/**
 * Account dashboard. Protected client-side: the session cookie is HttpOnly
 * and scoped to the API, so the page can't check it itself; once the session
 * state has loaded, signed-out visitors (including after signing out here)
 * are sent home. Every API call it makes is still checked by the Gateway.
 */
export default function ProfilePage() {
  const router = useRouter();
  const hasHydrated = useAuthStore((s) => s.hasHydrated);
  const userId = useAuthStore((s) => s.userId);
  const username = useAuthStore((s) => s.username);
  const email = useAuthStore((s) => s.email);
  const { list, ratings, status } = useUserLibrary();
  const logout = useLogout();

  useEffect(() => {
    if (hasHydrated && !username) router.replace("/");
  }, [hasHydrated, username, router]);

  if (!hasHydrated || !username || status === "idle" || status === "loading") return <ProfileSkeleton />;

  const recent = Object.values(ratings)
    .sort((a, b) => b.ratedAt - a.ratedAt)
    .slice(0, 5);

  return (
    <div className="relative flex-1 overflow-hidden">
      <div className="auth-aurora pointer-events-none absolute inset-x-0 top-0 h-80 opacity-50 blur-3xl" aria-hidden />

      <div className="relative mx-auto max-w-2xl space-y-6 px-4 py-10">
        {/* Identity */}
        <section className={`${GLASS} flex items-center gap-5 p-6 sm:p-8`}>
          <div className="flex size-20 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-primary to-amber-600 text-3xl font-bold text-zinc-950 shadow-lg shadow-primary/20">
            {username.charAt(0).toUpperCase()}
          </div>
          <div className="min-w-0">
            <h1 className="truncate text-2xl font-bold tracking-tight sm:text-3xl">{username}</h1>
            {email && <p className="truncate text-sm text-muted-foreground">{email}</p>}
          </div>
        </section>

        {/* Stats */}
        <section className="grid grid-cols-2 gap-4">
          <Stat href="/my-list" icon={<Bookmark className="size-5 text-primary" />} value={list.length} label="In My List" />
          <Stat icon={<Star className="size-5 text-primary" />} value={Object.keys(ratings).length} label="Ratings" />
        </section>

        {/* Recent ratings */}
        <section className={`${GLASS} p-6`}>
          <h2 className="text-lg font-bold tracking-tight">Recent ratings</h2>
          {recent.length === 0 ? (
            <p className="mt-3 text-sm text-muted-foreground">
              You haven&apos;t rated anything yet. Rate a movie from its page or like it in the{" "}
              <Link href="/discover" className="text-primary underline-offset-4 hover:underline">
                Discover feed
              </Link>
              .
            </p>
          ) : (
            <ul className="mt-4 divide-y divide-white/5">
              {recent.map(({ movie, score, ratedAt }) => {
                const poster = posterUrl(movie.poster_path, "w185");
                return (
                  <li key={movie.id}>
                    <Link href={`/movies/${movie.id}`} className="-mx-2 flex items-center gap-4 rounded-lg px-2 py-3 transition-colors hover:bg-white/5">
                      <div className="relative h-16 w-11 shrink-0 overflow-hidden rounded-md bg-zinc-900">
                        {poster && <Image src={poster} alt="" fill sizes="44px" className="object-cover" />}
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="truncate font-medium">{movie.title}</p>
                        <p className="text-xs text-muted-foreground">{relativeTime(new Date(ratedAt).toISOString())}</p>
                      </div>
                      <span className="flex items-center gap-1 text-sm font-semibold">
                        <Star className="size-4 fill-primary text-primary" />
                        {score}/10
                      </span>
                    </Link>
                  </li>
                );
              })}
            </ul>
          )}
        </section>

        {/* Account settings */}
        <section className={`${GLASS} p-6`}>
          <h2 className="text-lg font-bold tracking-tight">Account settings</h2>
          <div className="mt-4 divide-y divide-white/5">
            <SettingRow icon={<UserRound className="size-4" />} label="Username" value={username} />
            <SettingRow icon={<Mail className="size-4" />} label="Email" value={email ?? "Sign in again to show your email"} />
            <SettingRow
              icon={<KeyRound className="size-4" />}
              label="Password"
              value="••••••••"
              action={
                <Button variant="secondary" size="sm" disabled title="Coming soon">
                  Change
                </Button>
              }
            />
          </div>
          <Button variant="destructive" className="mt-6 w-full sm:w-auto" onClick={() => logout.mutate()} disabled={logout.isPending}>
            <LogOut className="size-4" />
            Sign out
          </Button>
        </section>

        {status === "error" && (
          <p className="px-2 text-center text-xs text-muted-foreground">
            Couldn&apos;t load your list and ratings.{" "}
            <button
              type="button"
              className="text-primary underline-offset-4 hover:underline"
              onClick={() => userId && void useLibraryStore.getState().load(userId)}
            >
              Try again
            </button>
          </p>
        )}
      </div>
    </div>
  );
}

function Stat({ icon, value, label, href }: { icon: ReactNode; value: number; label: string; href?: string }) {
  const body = (
    <>
      {icon}
      <p className="mt-3 text-3xl font-bold tracking-tight">{value}</p>
      <p className="text-sm text-muted-foreground">{label}</p>
    </>
  );
  return href ? (
    <Link href={href} className={`${GLASS} block p-5 transition-colors hover:border-white/20`}>
      {body}
    </Link>
  ) : (
    <div className={`${GLASS} p-5`}>{body}</div>
  );
}

function SettingRow({ icon, label, value, action }: { icon: ReactNode; label: string; value: string; action?: ReactNode }) {
  return (
    <div className="flex items-center gap-4 py-3">
      <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-white/5 text-muted-foreground">{icon}</span>
      <div className="min-w-0 flex-1">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="truncate text-sm font-medium">{value}</p>
      </div>
      {action}
    </div>
  );
}

function ProfileSkeleton() {
  return (
    <div className="mx-auto w-full max-w-2xl flex-1 space-y-6 px-4 py-10">
      <Skeleton className="h-32 w-full rounded-2xl" />
      <div className="grid grid-cols-2 gap-4">
        <Skeleton className="h-32 rounded-2xl" />
        <Skeleton className="h-32 rounded-2xl" />
      </div>
      <Skeleton className="h-48 w-full rounded-2xl" />
    </div>
  );
}
