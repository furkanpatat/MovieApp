"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { CircleCheck, UserX } from "lucide-react";

import { WatchedGrid } from "@/components/profile/watched-grid";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { usePublicProfile } from "@/hooks/queries";
import { useT } from "@/i18n";
import { ApiError } from "@/lib/api-client";
import { useAuthStore } from "@/store/auth-store";

const GLASS = "rounded-2xl border border-white/10 bg-zinc-950/60 shadow-2xl shadow-black/30 backdrop-blur-xl";

/**
 * A user's public profile, /u/{username}: their name and what they watched.
 * Anyone can open it (it's the link "Share profile" gives out); their list,
 * ratings and email stay private.
 */
export default function PublicProfilePage() {
  const { username: raw } = useParams<{ username: string }>();
  const username = decodeURIComponent(raw);
  const { t } = useT();
  const profile = usePublicProfile(username);
  const me = useAuthStore((s) => s.username);

  if (profile.status === "pending") {
    return (
      <div className="mx-auto w-full max-w-3xl space-y-6 px-4 py-10">
        <Skeleton className="h-32 w-full rounded-2xl" />
        <Skeleton className="h-64 w-full rounded-2xl" />
      </div>
    );
  }
  if (profile.status === "error") {
    const notFound = profile.error instanceof ApiError && profile.error.status === 404;
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-4 px-6 py-24 text-center">
        <UserX className="size-10 text-muted-foreground" strokeWidth={1.5} />
        <p className="text-lg font-semibold">{t(notFound ? "publicProfile.notFound" : "common.catalogUnreachable")}</p>
        {!notFound && (
          <Button variant="secondary" onClick={() => profile.refetch()}>
            {t("common.tryAgain")}
          </Button>
        )}
      </div>
    );
  }

  const { username: name, items } = profile.data;
  const mine = me !== null && me.toLowerCase() === name.toLowerCase();

  return (
    <div className="relative flex-1 overflow-hidden">
      <div className="auth-aurora pointer-events-none absolute inset-x-0 top-0 h-80 opacity-50 blur-3xl" aria-hidden />
      <div className="relative mx-auto w-full max-w-3xl space-y-6 px-4 py-10">
        <section className={`${GLASS} flex items-center gap-5 p-6 sm:p-8`}>
          <div className="flex size-20 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-primary to-amber-600 text-3xl font-bold text-zinc-950 shadow-lg shadow-primary/20">
            {name.charAt(0).toUpperCase()}
          </div>
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-2xl font-bold tracking-tight sm:text-3xl">{name}</h1>
            <p className="mt-1 flex items-center gap-1.5 text-sm text-muted-foreground">
              <CircleCheck className="size-4 text-primary" />
              {t("publicProfile.watchedCount", { n: items.length })}
            </p>
            {mine && (
              <p className="mt-3 text-xs text-muted-foreground">
                {t("publicProfile.yours")}{" "}
                <Link href="/profile" className="text-primary underline-offset-4 hover:underline">
                  {t("publicProfile.edit")}
                </Link>
              </p>
            )}
          </div>
        </section>

        <section className={`${GLASS} p-6`}>
          <h2 className="text-lg font-bold tracking-tight">{t("profile.watchedTitle")}</h2>
          {items.length === 0 ? (
            <p className="mt-4 text-sm text-muted-foreground">{t("publicProfile.empty", { name })}</p>
          ) : (
            <div className="mt-4">
              <WatchedGrid movies={items.map((w) => w.movie)} />
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
