"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { motion, useMotionValueEvent, useScroll } from "framer-motion";
import { Bookmark, Clapperboard, Compass, LogOut, Search, User, UserRound } from "lucide-react";

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
        <Link
          href="/"
          className={`group flex shrink-0 items-center gap-2 ${control}`}
          aria-label="MovieApp home"
        >
          <motion.span
            whileHover={{ rotate: -8, scale: 1.08 }}
            whileTap={{ scale: 0.95 }}
            transition={{ type: "spring", stiffness: 400, damping: 15 }}
            className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground"
          >
            <Clapperboard className="size-5" strokeWidth={2.25} />
          </motion.span>
          <span className="text-lg font-bold tracking-tight">
            Movie<span className="text-primary">App</span>
          </span>
        </Link>

        <div className={`hidden flex-1 items-center justify-center sm:flex ${control}`}>
          <SearchBox />
        </div>

        <div className={`flex shrink-0 items-center gap-2 ${control}`}>
          <Button variant="ghost" size="icon" className="sm:hidden" aria-label="Search" asChild>
            <Link href="/search">
              <Search className="size-5" />
            </Link>
          </Button>

          {/* The immersive video feed, one tap from anywhere. */}
          <Link
            href="/discover"
            aria-current={onDiscover ? "page" : undefined}
            className={`group flex items-center gap-2 rounded-full px-2.5 py-2 text-sm font-semibold transition-colors sm:px-3.5 ${
              onDiscover
                ? "bg-primary text-primary-foreground"
                : "bg-white/5 text-foreground ring-1 ring-white/10 hover:bg-white/10 hover:ring-primary/50"
            }`}
          >
            <Compass
              className={`size-5 transition-transform duration-500 group-hover:rotate-[135deg] ${onDiscover ? "" : "text-primary"}`}
            />
            <span className="hidden md:inline">Discover</span>
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
                  aria-label="Account menu"
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
                    Profile
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link href="/my-list">
                    <Bookmark className="size-4" />
                    My List
                    {listCount > 0 && (
                      <span className="ml-auto rounded-full bg-primary/15 px-1.5 text-xs font-semibold text-primary">
                        {listCount}
                      </span>
                    )}
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={() => logout.mutate()} variant="destructive">
                  <LogOut className="size-4" />
                  Sign out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          ) : (
            <Button size="sm" className="font-semibold" onClick={() => openAuth()}>
              <User className="size-4" />
              Sign in
            </Button>
          )}
        </div>
      </nav>
    </header>
  );
}
