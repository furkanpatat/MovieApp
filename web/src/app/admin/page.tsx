"use client";

import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Loader2, RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { useRequireAuth } from "@/hooks/use-library";
import { apiFetch } from "@/lib/api-client";
import { relativeTime } from "@/lib/format";
import { useAuthStore } from "@/store/auth-store";

const GLASS = "rounded-2xl border border-white/10 bg-zinc-950/60 shadow-2xl shadow-black/30 backdrop-blur-xl";

type Me = { user_id: string; admin: boolean };
type Report = {
  id: string;
  user_id: string; // the comment's author
  text: string;
  created_at: string;
  media_type: "movie" | "tv";
  movie_id: number;
  reports: number;
  last_reported_at: string;
};

/**
 * A plain admin panel: the reported comments, most reported first, each to
 * delete or to dismiss (keep the comment, clear the reports). Only the users
 * in the server's ADMIN_USER_IDS get in (the Gateway checks); anyone else
 * is shown their id, to be added there.
 */
export default function AdminPage() {
  const signedIn = useAuthStore((s) => s.hasHydrated && s.username !== null);
  const hydrated = useAuthStore((s) => s.hasHydrated);
  const userId = useAuthStore((s) => s.userId);
  const requireAuth = useRequireAuth();

  const me = useQuery({
    queryKey: ["admin", "me", userId],
    queryFn: () => apiFetch<Me>("/api/v1/admin/me"),
    enabled: signedIn,
  });

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6 px-4 py-10">
      <h1 className="text-3xl font-bold tracking-tight">Admin</h1>
      {!hydrated || (signedIn && me.isPending) ? (
        <Loader2 className="size-5 animate-spin text-muted-foreground" />
      ) : !signedIn ? (
        <div className={`${GLASS} space-y-4 p-6`}>
          <p className="text-sm text-muted-foreground">Sign in with an admin account to continue.</p>
          <Button onClick={() => requireAuth(() => {})}>Sign in</Button>
        </div>
      ) : me.isError ? (
        <p className="text-sm text-red-300">Couldn&apos;t load. Try again.</p>
      ) : !me.data?.admin ? (
        <div className={`${GLASS} space-y-3 p-6 text-sm`}>
          <p>This account isn&apos;t an admin.</p>
          <p className="text-muted-foreground">
            To make it one, add this id to <code>ADMIN_USER_IDS</code> in the server&apos;s <code>.env</code>, then
            restart the gateway:
          </p>
          <code className="block rounded-lg bg-white/5 px-3 py-2 break-all select-all">{me.data?.user_id}</code>
        </div>
      ) : (
        <Reports />
      )}
    </div>
  );
}

function Reports() {
  const queryClient = useQueryClient();
  const reports = useQuery({
    queryKey: ["admin", "reports"],
    queryFn: async () => (await apiFetch<{ items: Report[] }>("/api/v1/admin/reports")).items,
  });

  const act = useMutation({
    mutationFn: ({ id, action }: { id: string; action: "delete" | "dismiss" }) =>
      apiFetch<void>(`/api/v1/admin/comments/${id}/${action}`, { method: "POST" }),
    onSuccess: (_d, { action }) => {
      toast.success(action === "delete" ? "Comment deleted" : "Reports dismissed");
      void queryClient.invalidateQueries({ queryKey: ["admin", "reports"] });
    },
    onError: () => toast.error("That didn't work. Try again."),
  });

  const items = reports.data ?? [];
  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">
          Reported comments{reports.data ? ` (${items.length})` : ""}
        </h2>
        <Button size="sm" variant="secondary" onClick={() => void reports.refetch()} disabled={reports.isFetching}>
          <RefreshCw className={`size-4 ${reports.isFetching ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>

      {reports.isError && <p className="text-sm text-red-300">Couldn&apos;t load the reports.</p>}
      {reports.data && items.length === 0 && <p className={`${GLASS} p-6 text-sm text-muted-foreground`}>Nothing reported.</p>}

      <ul className="space-y-3">
        {items.map((r) => {
          const busy = act.isPending && act.variables?.id === r.id;
          return (
            <li key={r.id} className={`${GLASS} space-y-3 p-5`}>
              <p className="text-sm break-words whitespace-pre-wrap">{r.text}</p>
              <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
                <span className="font-semibold text-amber-300">
                  {r.reports} {r.reports === 1 ? "report" : "reports"}
                </span>
                <span>last {relativeTime(r.last_reported_at)}</span>
                <span>
                  by <code className="select-all">{r.user_id}</code>
                </span>
                <Link
                  href={r.media_type === "tv" ? `/tv/${r.movie_id}` : `/movies/${r.movie_id}`}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  on {r.media_type === "tv" ? "series" : "movie"} {r.movie_id}
                </Link>
                <span>posted {relativeTime(r.created_at)}</span>
              </div>
              <div className="flex gap-2">
                <Button
                  size="sm"
                  variant="destructive"
                  disabled={busy}
                  onClick={() => {
                    if (window.confirm("Delete this comment for good?")) act.mutate({ id: r.id, action: "delete" });
                  }}
                >
                  <Trash2 className="size-4" />
                  Delete comment
                </Button>
                <Button size="sm" variant="secondary" disabled={busy} onClick={() => act.mutate({ id: r.id, action: "dismiss" })}>
                  <Check className="size-4" />
                  Dismiss reports
                </Button>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
