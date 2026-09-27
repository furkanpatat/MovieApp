"use client";

import { useState, type ReactNode } from "react";
import { useT } from "@/i18n";
import { Loader2 } from "lucide-react";

import { Dialog, DialogContent, DialogTitle, DialogTrigger } from "@/components/ui/dialog";

/**
 * Full-width trailer modal. Opened by an explicit click, so unlike the muted
 * hover previews it autoplays *with* sound and keeps YouTube's controls: the
 * click counts as the user activation the browser needs, and allow="autoplay"
 * delegates it into the cross-origin iframe. Closing unmounts the iframe,
 * which is what actually stops playback.
 */
export function TrailerDialog({ videoKey, title, trigger }: { videoKey: string; title: string; trigger: ReactNode }) {
  const [loaded, setLoaded] = useState(false);
  const { t } = useT();

  return (
    <Dialog onOpenChange={(open) => !open && setLoaded(false)}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      {/* Solid overlay, no backdrop blur: Chrome can composite a cross-origin
          iframe above a backdrop-filter layer as a black box. */}
      <DialogContent
        overlayClassName="bg-black/85 supports-backdrop-filter:backdrop-blur-none"
        className="gap-0 overflow-hidden border-0 bg-black p-0 ring-white/10 sm:max-w-[min(64rem,calc(100%-2rem))]"
        // Radix would focus the first focusable child, the iframe; key
        // presses would then go to YouTube's cross-origin frame and Escape
        // would never close the dialog. Focus the dialog itself instead.
        onOpenAutoFocus={(e) => {
          e.preventDefault();
          (e.currentTarget as HTMLElement).focus();
        }}
      >
        <DialogTitle className="sr-only">{t("detail.trailerOf", { title })}</DialogTitle>
        <div className="relative aspect-video w-full">
          {!loaded && (
            <div className="absolute inset-0 flex items-center justify-center bg-zinc-950">
              <Loader2 className="size-10 animate-spin text-primary" aria-label={t("detail.loadingTrailer")} />
            </div>
          )}
          <iframe
            src={`https://www.youtube-nocookie.com/embed/${encodeURIComponent(videoKey)}?autoplay=1&playsinline=1&rel=0&modestbranding=1`}
            title={t("detail.trailerOf", { title })}
            allow="autoplay; encrypted-media; picture-in-picture; fullscreen"
            allowFullScreen
            onLoad={() => setLoaded(true)}
            className="absolute inset-0 size-full"
          />
        </div>
      </DialogContent>
    </Dialog>
  );
}
