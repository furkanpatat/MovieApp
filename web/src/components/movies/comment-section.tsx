"use client";

import { useState, type ReactNode } from "react";
import { Loader2, MessageCircle, SendHorizontal } from "lucide-react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import { useComment, useInteractions } from "@/hooks/queries";
import { useRequireAuth } from "@/hooks/use-library";
import { useAuthStore } from "@/store/auth-store";
import { displayName, idColor, initials, relativeTime } from "@/lib/format";
import type { OptimisticComment } from "@/hooks/queries";
import type { Interactions } from "@/types/movie";

const MAX_LEN = 1000;

/** Inline comments block (detail page): composer on top, then the thread. */
export function CommentSection({ movieId, interactions }: { movieId: number; interactions: Interactions }) {
  return (
    <div className="space-y-4">
      <h2 className="flex items-center gap-2 text-lg font-bold tracking-tight">
        <MessageCircle className="size-5 text-primary" />
        Comments
      </h2>
      <CommentComposer movieId={movieId} />
      <div className="max-h-[32rem] overflow-y-auto pr-1">
        <CommentList interactions={interactions} />
      </div>
    </div>
  );
}

/**
 * TikTok-style bottom sheet: the thread scrolls, the composer stays pinned
 * to the bottom. Reading is public; posting asks signed-out users to sign in.
 */
export function CommentSheet({ movieId, title, trigger }: { movieId: number; title: string; trigger: ReactNode }) {
  const { data: interactions } = useInteractions(movieId);
  const count = interactions?.recent_comments.length ?? 0;

  return (
    <Sheet>
      <SheetTrigger asChild>{trigger}</SheetTrigger>
      <SheetContent
        side="bottom"
        className="mx-auto flex h-[75dvh] max-w-2xl flex-col gap-0 rounded-t-2xl border-zinc-800 bg-zinc-950/95 p-0 backdrop-blur-xl"
      >
        <div className="mx-auto mt-2.5 h-1 w-10 shrink-0 rounded-full bg-white/20" aria-hidden />
        <SheetHeader className="border-b border-white/10 px-5 pt-3 pb-3 text-left">
          <SheetTitle className="text-white">
            {count} {count === 1 ? "comment" : "comments"}
          </SheetTitle>
          <SheetDescription className="line-clamp-1">{title}</SheetDescription>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-4">
          {interactions ? (
            <CommentList interactions={interactions} />
          ) : (
            <div className="space-y-4" aria-busy>
              {Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="h-14 animate-pulse rounded-lg bg-zinc-800/50" />
              ))}
            </div>
          )}
        </div>

        <div className="border-t border-white/10 bg-zinc-950 px-5 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
          <CommentComposer movieId={movieId} compact />
        </div>
      </SheetContent>
    </Sheet>
  );
}

function CommentComposer({ movieId, compact = false }: { movieId: number; compact?: boolean }) {
  const isAuthed = useAuthStore((s) => s.hasHydrated && s.username !== null);
  const requireAuth = useRequireAuth();
  const comment = useComment(movieId);
  const [text, setText] = useState("");

  const submit = () => {
    const trimmed = text.trim();
    if (!trimmed) return;
    comment.mutate(trimmed);
    setText("");
  };

  if (!isAuthed) {
    return (
      <Button variant="secondary" className={compact ? "w-full" : ""} onClick={() => requireAuth(() => {})}>
        Sign in to comment
      </Button>
    );
  }

  return (
    <div className="space-y-2">
      <Textarea
        placeholder="Add a comment…"
        value={text}
        maxLength={MAX_LEN}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) submit();
        }}
        className={`resize-none ${compact ? "min-h-12" : "min-h-20"}`}
      />
      <div className="flex items-center justify-between">
        <span className="text-xs text-muted-foreground">
          {text.length}/{MAX_LEN}
        </span>
        <Button size="sm" onClick={submit} disabled={!text.trim() || comment.isPending}>
          {comment.isPending ? <Loader2 className="size-4 animate-spin" /> : <SendHorizontal className="size-4" />}
          Post
        </Button>
      </div>
      {comment.isError && <p className="text-sm text-red-300">Couldn&apos;t post that comment. Please try again.</p>}
    </div>
  );
}

function CommentList({ interactions }: { interactions: Interactions }) {
  const currentUserId = useAuthStore((s) => s.userId);

  if (interactions.recent_comments.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">No comments yet — be the first to say something.</p>
    );
  }

  return (
    <div className="space-y-4">
      {interactions.recent_comments.map((c) => {
        const optimistic = c as OptimisticComment;
        return (
          <div key={c.id} className={`flex gap-3 ${optimistic.pending ? "opacity-60" : ""}`}>
            <Avatar className="size-8 shrink-0 border border-white/10">
              <AvatarFallback className="text-xs font-semibold text-white" style={{ backgroundColor: idColor(c.user_id) }}>
                {initials(c.user_id, currentUserId)}
              </AvatarFallback>
            </Avatar>
            <div className="min-w-0 flex-1">
              <div className="flex items-baseline gap-2">
                <span className="text-sm font-medium">{displayName(c.user_id, currentUserId)}</span>
                <span className="text-xs text-muted-foreground">
                  {optimistic.pending ? "sending…" : relativeTime(c.created_at)}
                </span>
              </div>
              <p className="mt-0.5 text-sm break-words text-foreground/90">{c.text}</p>
            </div>
          </div>
        );
      })}
    </div>
  );
}
