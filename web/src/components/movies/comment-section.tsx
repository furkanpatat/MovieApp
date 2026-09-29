"use client";

import { useState, useSyncExternalStore, type ReactNode } from "react";
import { AnimatePresence, motion, useDragControls } from "framer-motion";
import { ArrowUp, Ban, Flag, Loader2, MessageCircle, MoreHorizontal, SendHorizontal } from "lucide-react";
import { toast } from "sonner";
import { Dialog as DialogPrimitive } from "radix-ui";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Textarea } from "@/components/ui/textarea";
import { plural, useT } from "@/i18n";
import type { TitleRef } from "@/lib/media";
import { useBlockedUsers, useBlockUser, useComment, useInteractions, useReportComment } from "@/hooks/queries";
import { useRequireAuth } from "@/hooks/use-library";
import { useAuthStore } from "@/store/auth-store";
import { displayName, idColor, initials, relativeTime } from "@/lib/format";
import type { OptimisticComment } from "@/hooks/queries";
import type { Comment, Interactions } from "@/types/movie";

const MAX_LEN = 1000;

/** Inline comments block (detail page): composer on top, then the thread. */
export function CommentSection({ subject, interactions }: { subject: TitleRef; interactions: Interactions }) {
  const { t } = useT();
  return (
    <div className="space-y-4">
      <h2 className="flex items-center gap-2 text-lg font-bold tracking-tight">
        <MessageCircle className="size-5 text-primary" />
        {t("comments.title")}
      </h2>
      <CommentComposer subject={subject} />
      <div className="max-h-[32rem] overflow-y-auto pr-1">
        <CommentList interactions={interactions} />
      </div>
    </div>
  );
}

/** Pixels the on-screen keyboard covers at the bottom of the layout
 *  viewport (0 without one): `dvh` units don't shrink for the keyboard. */
function subscribeViewport(onChange: () => void) {
  const vv = window.visualViewport;
  vv?.addEventListener("resize", onChange);
  vv?.addEventListener("scroll", onChange);
  return () => {
    vv?.removeEventListener("resize", onChange);
    vv?.removeEventListener("scroll", onChange);
  };
}
function keyboardInset() {
  const vv = window.visualViewport;
  return vv ? Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop)) : 0;
}
const useKeyboardInset = () => useSyncExternalStore(subscribeViewport, keyboardInset, () => 0);

const SHEET_SPRING = { type: "spring", stiffness: 380, damping: 38, mass: 0.8 } as const;

/**
 * TikTok-style bottom sheet: it springs up over a soft, blurred overlay; the
 * thread scrolls and the pill composer stays pinned to the bottom, riding
 * above the on-screen keyboard. Drag the handle (or header) down, or flick
 * it, to dismiss. Reading is public; posting asks signed-out users to sign in.
 */
/** `subject` is the movie or series; `title` its name, shown in the header. */
export function CommentSheet({ subject, title, trigger }: { subject: TitleRef; title: string; trigger: ReactNode }) {
  const [open, setOpen] = useState(false);
  const { data: interactions } = useInteractions(subject, open);
  const blocked = useBlockedUsers().data;
  const count = interactions?.recent_comments.filter((c) => !blocked?.includes(c.user_id)).length ?? 0;
  const { t } = useT();
  const drag = useDragControls();
  const keyboard = useKeyboardInset();

  return (
    <DialogPrimitive.Root open={open} onOpenChange={setOpen}>
      <DialogPrimitive.Trigger asChild>{trigger}</DialogPrimitive.Trigger>
      <AnimatePresence>
        {open && (
          <DialogPrimitive.Portal forceMount>
            <DialogPrimitive.Overlay asChild forceMount>
              <motion.div
                className="fixed inset-0 z-50 bg-black/40 backdrop-blur-md"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                transition={{ duration: 0.25 }}
              />
            </DialogPrimitive.Overlay>
            <DialogPrimitive.Content asChild forceMount>
              <motion.div
                className="fixed inset-x-0 z-50 mx-auto flex h-[75dvh] max-w-2xl flex-col rounded-t-3xl border border-b-0 border-white/10 bg-zinc-950/80 shadow-2xl shadow-black/60 outline-none backdrop-blur-2xl"
                style={{ bottom: keyboard, maxHeight: `calc(100svh - ${keyboard}px - 1rem)` }}
                initial={{ y: "100%" }}
                animate={{ y: 0 }}
                exit={{ y: "100%" }}
                transition={SHEET_SPRING}
                drag="y"
                dragControls={drag}
                dragListener={false}
                dragConstraints={{ top: 0, bottom: 0 }}
                dragElastic={{ top: 0, bottom: 0.7 }}
                onDragEnd={(_, info) => {
                  if (info.offset.y > 120 || info.velocity.y > 600) setOpen(false);
                }}
              >
                {/* The grab area: handle and header drag the sheet; the list scrolls. */}
                <div
                  className="shrink-0 cursor-grab touch-none active:cursor-grabbing"
                  onPointerDown={(e) => drag.start(e)}
                >
                  <div className="mx-auto mt-2.5 h-1 w-10 rounded-full bg-white/25" aria-hidden />
                  <div className="px-5 pt-3 pb-3 text-center">
                    <DialogPrimitive.Title className="text-sm font-semibold text-white">
                      {interactions ? t(plural(count, "comments.countOne", "comments.countOther"), { n: count }) : t("comments.title")}
                    </DialogPrimitive.Title>
                    <DialogPrimitive.Description className="mt-0.5 line-clamp-1 text-xs text-zinc-400">
                      {title}
                    </DialogPrimitive.Description>
                  </div>
                  <div className="h-px bg-gradient-to-r from-transparent via-white/10 to-transparent" />
                </div>

                <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-4">
                  {interactions ? (
                    <CommentList interactions={interactions} />
                  ) : (
                    <div className="space-y-4" aria-busy>
                      {Array.from({ length: 4 }).map((_, i) => (
                        <div key={i} className="h-14 animate-pulse rounded-2xl bg-white/5" />
                      ))}
                    </div>
                  )}
                </div>

                <div className="shrink-0 px-3 pt-2 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
                  <PillComposer subject={subject} />
                </div>
              </motion.div>
            </DialogPrimitive.Content>
          </DialogPrimitive.Portal>
        )}
      </AnimatePresence>
    </DialogPrimitive.Root>
  );
}

/** The sheet's composer: a rounded iOS/TikTok-style field with a send
 *  button that pops in once there is something to send. Enter sends,
 *  Shift+Enter adds a line. */
function PillComposer({ subject }: { subject: TitleRef }) {
  const { t } = useT();
  const isAuthed = useAuthStore((s) => s.hasHydrated && s.username !== null);
  const currentUserId = useAuthStore((s) => s.userId);
  const requireAuth = useRequireAuth();
  const comment = useComment(subject);
  const [text, setText] = useState("");
  const ready = text.trim().length > 0 && !comment.isPending;

  const submit = () => {
    const trimmed = text.trim();
    if (!trimmed || comment.isPending) return;
    comment.mutate(trimmed);
    setText("");
  };

  if (!isAuthed) {
    return (
      <button
        type="button"
        onClick={() => requireAuth(() => {})}
        className="w-full rounded-full border border-white/10 bg-white/[0.06] px-5 py-3 text-left text-sm text-zinc-400 transition-colors hover:bg-white/10"
      >
        {t("comments.signInToAdd")}
      </button>
    );
  }

  return (
    <div>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
        className="flex items-end gap-2"
      >
        <Avatar className="mb-0.5 size-9 shrink-0 border border-white/10">
          <AvatarFallback className="text-xs font-semibold text-white" style={{ backgroundColor: idColor(currentUserId ?? "") }}>
            {initials(currentUserId ?? "", currentUserId)}
          </AvatarFallback>
        </Avatar>
        <div className="flex min-w-0 flex-1 items-end rounded-[1.4rem] border border-white/10 bg-white/[0.06] py-1 pr-1 pl-4 transition-colors focus-within:border-white/25 focus-within:bg-white/[0.08]">
          <textarea
            value={text}
            maxLength={MAX_LEN}
            rows={1}
            placeholder={t("comments.add")}
            aria-label={t("comments.add")}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                e.preventDefault();
                submit();
              }
            }}
            className="field-sizing-content max-h-28 min-h-9 flex-1 resize-none bg-transparent py-2 text-sm text-white outline-none placeholder:text-zinc-500"
          />
          <AnimatePresence initial={false}>
            {ready && (
              <motion.button
                type="submit"
                aria-label={t("comments.postComment")}
                initial={{ scale: 0, opacity: 0 }}
                animate={{ scale: 1, opacity: 1 }}
                exit={{ scale: 0, opacity: 0 }}
                transition={{ type: "spring", stiffness: 500, damping: 28 }}
                whileTap={{ scale: 0.88 }}
                className="mb-0.5 flex size-8 shrink-0 items-center justify-center rounded-full bg-primary text-zinc-950 shadow-lg shadow-primary/30"
              >
                <ArrowUp className="size-4" strokeWidth={2.5} />
              </motion.button>
            )}
          </AnimatePresence>
          {comment.isPending && <Loader2 className="mb-2 mr-2 size-4 shrink-0 animate-spin text-zinc-400" />}
        </div>
      </form>
      {text.length > MAX_LEN - 200 && (
        <p className="mt-1 pr-3 text-right text-[11px] text-zinc-500 tabular-nums">
          {text.length}/{MAX_LEN}
        </p>
      )}
      {comment.isError && <p className="mt-1.5 pl-12 text-xs text-red-300">{t("comments.failed")}</p>}
    </div>
  );
}

/** The inline composer (detail page). */
function CommentComposer({ subject }: { subject: TitleRef }) {
  const { t } = useT();
  const isAuthed = useAuthStore((s) => s.hasHydrated && s.username !== null);
  const requireAuth = useRequireAuth();
  const comment = useComment(subject);
  const [text, setText] = useState("");

  const submit = () => {
    const trimmed = text.trim();
    if (!trimmed) return;
    comment.mutate(trimmed);
    setText("");
  };

  if (!isAuthed) {
    return (
      <Button variant="secondary" onClick={() => requireAuth(() => {})}>
        {t("comments.signInToComment")}
      </Button>
    );
  }

  return (
    <div className="space-y-2">
      <Textarea
        placeholder={t("comments.add")}
        value={text}
        maxLength={MAX_LEN}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) submit();
        }}
        className="min-h-20 resize-none"
      />
      <div className="flex items-center justify-between">
        <span className="text-xs text-muted-foreground">
          {text.length}/{MAX_LEN}
        </span>
        <Button size="sm" onClick={submit} disabled={!text.trim() || comment.isPending}>
          {comment.isPending ? <Loader2 className="size-4 animate-spin" /> : <SendHorizontal className="size-4" />}
          {t("comments.post")}
        </Button>
      </div>
      {comment.isError && <p className="text-sm text-red-300">{t("comments.failed")}</p>}
    </div>
  );
}

function CommentList({ interactions }: { interactions: Interactions }) {
  const { t, locale } = useT();
  const currentUserId = useAuthStore((s) => s.userId);
  const blocked = new Set(useBlockedUsers().data ?? []);
  const comments = interactions.recent_comments.filter((c) => !blocked.has(c.user_id));

  if (comments.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">{t("comments.empty")}</p>
    );
  }

  return (
    <div className="space-y-4">
      {comments.map((c) => {
        const optimistic = c as OptimisticComment;
        return (
          <div key={c.id} className={`flex gap-3 ${optimistic.pending ? "opacity-60" : ""}`}>
            <Avatar className="size-8 shrink-0 border border-white/10">
              <AvatarFallback className="text-xs font-semibold text-white" style={{ backgroundColor: idColor(c.user_id) }}>
                {initials(c.user_id, currentUserId, locale)}
              </AvatarFallback>
            </Avatar>
            <div className="min-w-0 flex-1">
              <div className="flex items-baseline gap-2">
                <span className="text-sm font-medium">{displayName(c.user_id, currentUserId, locale)}</span>
                <span className="text-xs text-muted-foreground">
                  {optimistic.pending ? t("comments.sending") : relativeTime(c.created_at, locale)}
                </span>
              </div>
              <p className="mt-0.5 text-sm break-words text-foreground/90">{c.text}</p>
            </div>
            {!optimistic.pending && c.user_id !== currentUserId && <CommentMenu comment={c} />}
          </div>
        );
      })}
    </div>
  );
}

/** Report a comment, or block its author (hiding all their comments). */
function CommentMenu({ comment }: { comment: Comment }) {
  const { t } = useT();
  const report = useReportComment();
  const blockUser = useBlockUser();
  const requireAuth = useRequireAuth(); // signed out: asks to sign in first

  const onReport = () =>
    requireAuth(() =>
      report.mutate(comment.id, {
        onSuccess: () => toast.success(t("comments.reported")),
        onError: () => toast.error(t("comments.reportFailed")),
      }),
    );
  const onBlock = () =>
    requireAuth(() =>
      blockUser.mutate(
        { target: comment.user_id, block: true },
        {
          onSuccess: () =>
            toast(t("comments.blocked"), {
              action: { label: t("comments.undo"), onClick: () => blockUser.mutate({ target: comment.user_id, block: false }) },
            }),
          onError: () => toast.error(t("comments.blockFailed")),
        },
      ),
    );

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label={t("comments.more")}
        className="flex size-8 shrink-0 items-center justify-center rounded-full text-zinc-500 transition-colors hover:bg-white/10 hover:text-white focus-visible:outline-2"
      >
        <MoreHorizontal className="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-max">
        <DropdownMenuItem onSelect={onReport}>
          <Flag /> {t("comments.report")}
        </DropdownMenuItem>
        <DropdownMenuItem variant="destructive" onSelect={onBlock}>
          <Ban /> {t("comments.block")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
