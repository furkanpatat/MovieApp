"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { motion, useMotionValueEvent, useScroll } from "framer-motion";
import { Bookmark, Compass, House, Languages, LogOut, Search, User, UserRound } from "lucide-react";

import { LanguageToggle } from "@/components/layout/language-toggle";
import { ModeSwitcher } from "@/components/layout/mode-switcher";
import { useT } from "@/i18n";
import { SearchBox } from "@/components/layout/search-box";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useLogout } from "@/hooks/use-auth";
import { useAuthPrompt } from "@/store/auth-prompt-store";
import { useAuthStore } from "@/store/auth-store";
import { useLibraryStore } from "@/store/library-store";

type NavMode = "immersive" | "overlay" | "solid";

function navModeFor(pathname: string): NavMode {
  if (pathname === "/discover" || pathname.startsWith("/discover/")) return "immersive";
  if (pathname === "/") return "overlay";
  return "solid";
}

/**
 * Persistent top navigation, in one of three modes:
 * - Immersive (the /discover video feed): fixed and floating over the video
 *   on a fading gradient, controls dimmed until hovered. The header itself
 *   ignores pointer events so taps on the video beneath still land; only the
 *   control groups opt back in.
 * - Overlay (home): fixed over the hero art, transparent at the top, gaining
 *   a blurred dark backdrop once the page scrolls. The threshold is tracked as
 *   a single boolean and the blur/colour change is a CSS transition, so
 *   steady-state scrolling does no per-frame JS style work.
 * - Solid (everywhere else): a solid sticky bar that takes up layout space,
 *   so page content never slides under it.
 */
export function TopNav() {
  const mode = navModeFor(usePathname());
  const immersive = mode === "immersive";

  const [scrolled, setScrolled] = useState(false);
  const { scrollY } = useScroll();
  useMotionValueEvent(scrollY, "change", (latest) => setScrolled(latest > 8));
  // Dimmed until hovered or keyboard-focused; interactive despite the header's
  // pointer-events-none.
  const control = immersive
    ? "pointer-events-auto opacity-60 transition-opacity hover:opacity-100 focus-within:opacity-100"
    : "";

  const hasHydrated = useAuthStore((s) => s.hasHydrated);
  const username = useAuthStore((s) => s.username);
  const email = useAuthStore((s) => s.email);
  const listCount = useLibraryStore((s) => s.list.length);
  const openAuth = useAuthPrompt((s) => s.openAuth);
  const logout = useLogout();
  const isAuthed = hasHydrated && username !== null;
  const pathname = usePathname();
  const { t } = useT();
  const onDiscover = pathname === "/discover" || pathname.startsWith("/discover/");

  return (
    <header
      className={
        mode === "immersive"
          ? "pointer-events-none fixed left-0 top-0 z-50 w-full bg-gradient-to-b from-black/80 via-black/20 to-transparent pb-8"
          : mode === "overlay"
            ? `fixed left-0 top-0 z-50 w-full transition-colors duration-300 ${
                scrolled ? "bg-background/80 backdrop-blur-md" : "bg-gradient-to-b from-background/70 to-transparent"
              }`
            : "sticky top-0 z-50 w-full bg-background"
      }
    >
      <nav className="mx-auto flex h-16 max-w-screen-2xl items-center justify-between gap-4 px-4 sm:px-6 lg:px-8">
        {/* The logo doubles as the Movies <-> Series switch. */}
        <ModeSwitcher className={control} />

        <div className={`hidden flex-1 items-center justify-center sm:flex ${control}`}>
          <SearchBox />
        </div>

        <div className={`flex shrink-0 items-center gap-2 ${control}`}>
          <Button variant="ghost" size="icon" className="sm:hidden" aria-label={t("nav.search")} asChild>
            <Link href="/search">
              <Search className="size-5" />
            </Link>
          </Button>

          {/* Home, now that the logo is the Movies/Series switch rather than a
              home link. (On Discover the Discover button itself leads out.) */}
          {pathname !== "/" && !onDiscover && (
            <Button variant="ghost" size="icon" aria-label={t("nav.home")} title={t("nav.home")} asChild>
              <Link href="/">
                <House className="size-5" />
              </Link>
            </Button>
          )}

          {/* The immersive video feed, one tap from anywhere; tapped again
              while on it, it leads back home (the way out of the feed). */}
          <Link
            href={onDiscover ? "/" : "/discover"}
            aria-label={onDiscover ? t("nav.leaveDiscover") : undefined}
            title={onDiscover ? t("nav.leaveDiscover") : undefined}
            className={`group flex items-center gap-2 rounded-full px-2.5 py-2 text-sm font-semibold transition-colors sm:px-3.5 ${
              onDiscover
                ? "bg-primary text-primary-foreground"
                : "bg-white/5 text-foreground ring-1 ring-white/10 hover:bg-white/10 hover:ring-primary/50"
            }`}
          >
            {onDiscover ? (
              <House className="size-5" />
            ) : (
              <Compass className="size-5 text-primary transition-transform duration-500 group-hover:rotate-[135deg]" />
            )}
            <span className="hidden md:inline">{t(onDiscover ? "nav.home" : "nav.discover")}</span>
          </Link>

          {!hasHydrated ? (
            // Avoid rendering a guess before we know whether there's a
            // session: a neutral placeholder keeps layout stable and skips
            // any server/client mismatch.
            <div className="size-9 rounded-full bg-white/5" aria-hidden />
          ) : isAuthed ? (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <motion.button
                  whileHover={{ scale: 1.06 }}
                  whileTap={{ scale: 0.96 }}
                  transition={{ type: "spring", stiffness: 400, damping: 15 }}
                  className="rounded-full outline-none ring-primary/60 focus-visible:ring-2"
                  aria-label={t("nav.accountMenu")}
                >
                  <Avatar className="size-9 border border-white/10">
                    <AvatarFallback className="bg-secondary text-sm font-semibold text-secondary-foreground">
                      {username?.slice(0, 2).toUpperCase()}
                    </AvatarFallback>
                  </Avatar>
                </motion.button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-60">
                <DropdownMenuLabel className="font-normal">
                  <p className="truncate text-sm font-semibold text-foreground">{username}</p>
                  {email && <p className="truncate text-xs text-muted-foreground">{email}</p>}
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild>
                  <Link href="/profile">
                    <UserRound className="size-4" />
                    {t("nav.profile")}
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link href="/my-list">
                    <Bookmark className="size-4" />
                    {t("nav.myList")}
                    {listCount > 0 && (
                      <span className="ml-auto rounded-full bg-primary/15 px-1.5 text-xs font-semibold text-primary">
                        {listCount}
                      </span>
                    )}
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                {/* The language lives here and on the profile page, not in the bar. */}
                <div className="flex items-center justify-between gap-3 px-2 py-1.5 text-sm">
                  <span className="flex items-center gap-2 text-muted-foreground">
                    <Languages className="size-4" />
                    {t("nav.language")}
                  </span>
                  <LanguageToggle />
                </div>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={() => logout.mutate()} variant="destructive">
                  <LogOut className="size-4" />
                  {t("nav.signOut")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          ) : (
            <Button size="sm" className="font-semibold" onClick={() => openAuth()}>
              <User className="size-4" />
              {t("common.signIn")}
            </Button>
          )}
        </div>
      </nav>
    </header>
  );
}
