"use client";

import { useEffect, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { AnimatePresence, motion } from "framer-motion";
import { Dialog as DialogPrimitive } from "radix-ui";
import { X } from "lucide-react";

import { useT } from "@/i18n";
import { useTitleModalStore } from "@/store/modal-store";

/**
 * The glass modal a movie or series opens in over the current page, via the
 * @modal intercepting routes: the page behind never unmounts, so opening is
 * instant, and the URL is the title's own (shareable; a reload shows the
 * full page instead).
 *
 * Radix Dialog handles focus, Escape, outside clicks and scroll locking (and
 * stacks the trailer and comment dialogs above it); Framer Motion animates.
 * Closing plays the exit animation, then goes back in history, which
 * restores the page underneath exactly as it was.
 */
export function TitleModal({ label, children }: { label: string; children: ReactNode }) {
  const router = useRouter();
  const [open, setOpen] = useState(true);
  const setModalOpen = useTitleModalStore((s) => s.setOpen);
  const { t } = useT();

  useEffect(() => {
    setModalOpen(true);
    return () => setModalOpen(false);
  }, [setModalOpen]);

  return (
    <DialogPrimitive.Root open={open} onOpenChange={(o) => !o && setOpen(false)}>
      <AnimatePresence onExitComplete={() => router.back()}>
        {open && (
          <DialogPrimitive.Portal forceMount>
            <DialogPrimitive.Overlay asChild forceMount>
              <motion.div
                className="fixed inset-0 z-50 bg-black/60 backdrop-blur-md"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                transition={{ duration: 0.25, ease: "easeOut" }}
              />
            </DialogPrimitive.Overlay>
            <DialogPrimitive.Content asChild forceMount aria-describedby={undefined}>
              <motion.div
                className="fixed inset-x-0 top-[3vh] bottom-0 z-50 mx-auto flex w-full max-w-6xl flex-col overflow-hidden rounded-t-3xl border border-white/10 bg-zinc-950/85 shadow-2xl shadow-black/70 outline-none backdrop-blur-3xl sm:inset-x-4 sm:top-[2.5vh] sm:bottom-[2.5vh] sm:rounded-3xl"
                initial={{ opacity: 0, scale: 0.95, y: 16 }}
                animate={{ opacity: 1, scale: 1, y: 0 }}
                exit={{ opacity: 0, scale: 0.96, y: 10 }}
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
