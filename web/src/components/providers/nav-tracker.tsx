"use client";

import { usePathname } from "next/navigation";
import { useEffect } from "react";

import { recordPath } from "@/lib/nav-history";

/** Keeps lib/nav-history's record of route changes (for browsers without
 *  the Navigation API). Renders nothing. */
export function NavTracker() {
  const pathname = usePathname();
  useEffect(() => recordPath(pathname), [pathname]);
  return null;
}
