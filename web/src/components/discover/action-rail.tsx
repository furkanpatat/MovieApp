"use client";

import { forwardRef, type ComponentProps, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { Bookmark, BookmarkCheck, Heart, Info, MessageCircle, MoreHorizontal, Share2, Users, Volume2, VolumeX } from "lucide-react";
import { DropdownMenu as MenuPrimitive } from "radix-ui";
import { toast } from "sonner";

import { CommentSheet } from "@/components/movies/comment-section";
import { useT } from "@/i18n";
import { compactNumber } from "@/lib/format";
import { isTV, titleHref } from "@/lib/media";
import { newPartyCode, partyHref } from "@/lib/party";
import { cn } from "@/lib/utils";
import type { Movie } from "@/types/movie";

/** A round glass button on the rail, with an optional caption under it. */
const RailButton = forwardRef<HTMLButtonElement, ComponentProps<typeof motion.button> & { label: string; caption?: ReactNode }>(
  function RailButton({ label, caption, className, children, ...props }, ref) {
    return (
      <div className="flex flex-col items-center gap-1">
        <motion.button
          ref={ref}
          type="button"
          aria-label={label}
          whileTap={{ scale: 0.86 }}
          transition={{ type: "spring", stiffness: 500, damping: 25 }}
          className={cn(
            "flex size-12 items-center justify-center rounded-full bg-black/35 text-white ring-1 ring-white/10 backdrop-blur-md transition-colors hover:bg-black/55 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary",
            className,
          )}
          {...props}
        >
          {children}
        </motion.button>
        {caption !== undefined && <span className="text-xs font-semibold text-white tabular-nums drop-shadow-md">{caption}</span>}
      </div>
    );
  },
);

/**
 * The Discover post's actions: the four everyday ones on the rail (sound,
 * like, comments, save), everything else behind "more" so the video stays
 * the focus.
 */
export function ActionRail({
  movie,
  muted,
  onToggleMute,
  liked,
  likes,
  onLike,
  comments,
  saved,
  onToggleSaved,
}: {
  movie: Movie;
  muted: boolean;
  onToggleMute: () => void;
  liked: boolean;
  likes: number;
  onLike: () => void;
  comments: number;
  saved: boolean;
  onToggleSaved: () => void;
}) {
  const { t, locale } = useT();
  const compact = (n: number) => compactNumber(n, locale);
  const icon = "size-6";
  return (
    <div className="absolute right-3 bottom-6 z-20 flex flex-col items-center gap-4 sm:right-6 sm:bottom-8">
      <RailButton label={t(muted ? "discover.unmute" : "discover.mute")} onClick={(e) => (e.stopPropagation(), onToggleMute())}>
        {muted ? <VolumeX className={icon} /> : <Volume2 className={icon} />}
      </RailButton>

      <RailButton label={t("discover.like")} aria-pressed={liked} caption={compact(likes)} onClick={(e) => (e.stopPropagation(), onLike())}>
        {/* Re-keyed on change so the heart pops when it fills. */}
        <motion.span
          key={String(liked)}
          initial={{ scale: liked ? 0.5 : 1 }}
          animate={{ scale: 1 }}
          transition={{ type: "spring", stiffness: 520, damping: 14 }}
        >
          <Heart className={cn(icon, liked && "fill-primary text-primary")} />
        </motion.span>
      </RailButton>

      <CommentSheet
        subject={movie}
        title={movie.title}
        trigger={
          <RailButton label={t("discover.comments")} caption={compact(comments)}>
            <MessageCircle className={icon} />
          </RailButton>
        }
      />

      <RailButton
        label={t(saved ? "discover.removeFromList" : "discover.addToList")}
        aria-pressed={saved}
        caption={t(saved ? "discover.saved" : "discover.save")}
        onClick={(e) => (e.stopPropagation(), onToggleSaved())}
      >
        {saved ? <BookmarkCheck className={cn(icon, "text-primary")} /> : <Bookmark className={icon} />}
      </RailButton>

      <MoreMenu movie={movie} />
    </div>
  );
}

/** Details, Watch Party and Share, in a glass menu opening to the left. */
function MoreMenu({ movie }: { movie: Movie }) {
  const router = useRouter();
  const { t } = useT();

  const tv = isTV(movie);
  const share = async () => {
    const url = `${window.location.origin}${titleHref(movie)}`;
    try {
      if (navigator.share) {
        await navigator.share({ title: movie.title, url });
        return;
      }
      await navigator.clipboard.writeText(url);
      toast.success(t("discover.linkCopied"), { id: "share-link" });
    } catch (err) {
      // The user closing the share sheet isn't an error worth a toast.
      if (err instanceof DOMException && err.name === "AbortError") return;
      toast.error(t(tv ? "discover.shareFailedSeries" : "discover.shareFailedMovie"), { id: "share-link" });
    }
  };

  return (
    <MenuPrimitive.Root modal={false}>
      <MenuPrimitive.Trigger asChild>
        <RailButton label={t("discover.more")} className="size-10 bg-black/25">
          <MoreHorizontal className="size-5" />
        </RailButton>
      </MenuPrimitive.Trigger>
      <MenuPrimitive.Portal>
        <MenuPrimitive.Content
          side="left"
          align="end"
          sideOffset={12}
          onClick={(e) => e.stopPropagation()}
          className="z-50 min-w-48 origin-(--radix-dropdown-menu-content-transform-origin) rounded-2xl border border-white/10 bg-zinc-950/70 p-1.5 text-sm text-zinc-100 shadow-2xl shadow-black/60 backdrop-blur-2xl data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95"
        >
          <MenuItem icon={Info} onSelect={() => router.push(titleHref(movie))}>
            {t("discover.details")}
          </MenuItem>
          {!tv && (
            <MenuItem
              icon={Users}
              // A fresh private party: the movie opens (in the modal) and
              // joins it; its panel has the invite link to share.
              onSelect={() => router.push(partyHref(movie.id, newPartyCode()))}
            >
              {t("discover.watchParty")}
            </MenuItem>
          )}
          <MenuPrimitive.Separator className="mx-2 my-1 h-px bg-white/10" />
          <MenuItem icon={Share2} onSelect={share}>
            {t("discover.share")}
          </MenuItem>
        </MenuPrimitive.Content>
      </MenuPrimitive.Portal>
    </MenuPrimitive.Root>
  );
}

function MenuItem({ icon: Icon, children, onSelect }: { icon: typeof Info; children: ReactNode; onSelect: () => void }) {
  return (
    <MenuPrimitive.Item
      onSelect={onSelect}
      className="flex cursor-pointer items-center gap-3 rounded-xl px-3 py-2.5 font-medium outline-none select-none data-highlighted:bg-white/10 data-highlighted:text-white"
    >
      <Icon className="size-4 text-zinc-400" />
      {children}
    </MenuPrimitive.Item>
  );
}
