"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { useT } from "@/i18n";

/**
 * The footer: the TMDB attribution their API terms require (the notice and
 * their logo, less prominent than ours) and the legal links. Not on
 * Discover, a full-screen feed.
 */
export function SiteFooter() {
  const pathname = usePathname();
  const { t } = useT();
  if (pathname.startsWith("/discover")) return null;
  return (
    <footer className="border-t border-white/5 px-4 py-8 text-xs text-muted-foreground">
      <div className="mx-auto flex max-w-6xl flex-col items-center gap-4 text-center sm:flex-row sm:justify-between sm:text-left">
        <a href="https://www.themoviedb.org/" target="_blank" rel="noopener noreferrer" className="flex items-center gap-3 opacity-80 transition-opacity hover:opacity-100">
          <Image src="/tmdb-logo.svg" alt="TMDB" width={96} height={12} className="h-3 w-auto" unoptimized />
          <span className="max-w-md">{t("footer.tmdb")}</span>
        </a>
        <nav className="flex items-center gap-4">
          <Link href="/privacy" className="hover:text-foreground">
            {t("footer.privacy")}
          </Link>
          <span>© {new Date().getFullYear()} KinoCut</span>
        </nav>
      </div>
    </footer>
  );
}
