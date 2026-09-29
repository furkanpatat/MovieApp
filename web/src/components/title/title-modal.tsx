"use client";

import { useEffect, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { AnimatePresence, motion } from "framer-motion";
import { Dialog as DialogPrimitive } from "radix-ui";
import { X } from "lucide-react";

import { useT } from "@/i18n";
import { isModalRoute, previousPath } from "@/lib/nav-history";
import { useTitleModalStore } from "@/store/modal-store";

/**
 * The glass modal a movie or series opens in over the current page, via the
 * @modal intercepting routes: the page behind never unmounts, so opening is
 * instant, and the URL is the title's own (shareable; a reload shows the
 * full page instead).
 *
 * Radix Dialog handles focus, Escape, outside clicks and scroll locking (and
 * stacks the trailer and comment dialogs above it); Framer Motion animates.
 * Closing plays a short exit animation, then goes back in history, which
 * restores the page underneath exactly as it was.
 *
 * Modal to modal (a cast member from a title, a title from a person) swaps
 * the modal in the @modal slot: only one is ever on screen, and history
 * keeps the rest (Next keeps the previous one's state, so going back shows
 * it as it was, with no entrance). Closing such a stacked modal goes back
 * at once rather than waiting for its exit animation.
 */


/** A modal mounting before this was revealed by going back (ours or the
 *  browser's): it shows without an entrance, as if it never left. */
let revealUntil = 0;
if (typeof window !== "undefined") {
  window.addEventListener("popstate", () => {
    revealUntil = Date.now() + 1000;
  });
}

export function TitleModal({ label, children }: { label: string; children: ReactNode }) {
  const router = useRouter();
  const [open, setOpen] = useState(true);
  const [revealed] = useState(() => Date.now() < revealUntil);
  const setModalOpen = useTitleModalStore((s) => s.setOpen);
  const { t } = useT();

  useEffect(() => {
    setModalOpen(true);
    revealUntil = 0;
    return () => setModalOpen(false);
  }, [setModalOpen]);

  // Over another modal: back at once (it shows as it was). Over a page: the
  // exit animation, then back (AnimatePresence's onExitComplete).
  const close = () => {
    if (isModalRoute(previousPath())) {
      revealUntil = Date.now() + 1000;
      router.back();
    } else {
      setOpen(false);
    }
  };

  return (
    <DialogPrimitive.Root open={open} onOpenChange={(o) => !o && close()}>
      <AnimatePresence onExitComplete={() => router.back()}>
        {open && (
          <DialogPrimitive.Portal forceMount>
            <DialogPrimitive.Overlay asChild forceMount>
              <motion.div
                className="fixed inset-0 z-50 bg-black/60 backdrop-blur-md"
                initial={revealed ? false : { opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0, transition: { duration: 0.18, ease: "easeIn" } }}
                transition={{ duration: 0.25, ease: "easeOut" }}
              />
            </DialogPrimitive.Overlay>
            <DialogPrimitive.Content asChild forceMount aria-describedby={undefined}>
              <motion.div
                className="fixed inset-x-0 top-[3vh] bottom-0 z-50 mx-auto flex w-full max-w-6xl flex-col overflow-hidden rounded-t-3xl border border-white/10 bg-zinc-950/85 shadow-2xl shadow-black/70 outline-none backdrop-blur-3xl sm:inset-x-4 sm:top-[2.5vh] sm:bottom-[2.5vh] sm:rounded-3xl"
                initial={revealed ? false : { opacity: 0, scale: 0.95, y: 16 }}
                animate={{ opacity: 1, scale: 1, y: 0 }}
                exit={{ opacity: 0, scale: 0.97, y: 8, transition: { duration: 0.18, ease: "easeIn" } }}
                transition={{ type: "spring", stiffness: 360, damping: 32, mass: 0.9 }}
              >
                <DialogPrimitive.Title className="sr-only">{label}</DialogPrimitive.Title>
                {/* The details scroll inside the frame; the page behind stays put. */}
                <div className="flex min-h-0 flex-1 flex-col overflow-y-auto overscroll-contain">{children}</div>
                <DialogPrimitive.Close
                  aria-label={t("common.close")}
                  className="absolute top-4 right-4 z-20 flex size-10 items-center justify-center rounded-full bg-black/50 text-white ring-1 ring-white/15 backdrop-blur-md transition hover:bg-black/70 focus-visible:ring-2 focus-visible:ring-primary focus-visible:outline-none"
                >
                  <X className="size-5" />
                </DialogPrimitive.Close>
              </motion.div>
            </DialogPrimitive.Content>
          </DialogPrimitive.Portal>
        )}
      </AnimatePresence>
    </DialogPrimitive.Root>
  );
}
